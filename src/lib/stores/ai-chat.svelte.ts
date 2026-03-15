import { addToast } from './toast.svelte.js';

export interface AiStreamingState {
  text: string;
  toolCalls: Array<{ id: string; name: string; result?: string; is_error?: boolean }>;
}

class AiChatStore {
  sessions = $state<{ id: number; uuid: string; title: string; created_at: string; updated_at: string }[]>([]);
  activeSessionId = $state<number | null>(null);
  messages = $state<{ id: number; uuid: string; session_id: number; role: 'user' | 'assistant'; content: string; tool_calls: string | null; tool_results: string | null; is_streaming: number; created_at: string }[]>([]);
  streaming = $state<AiStreamingState>({ text: '', toolCalls: [] });
  isStreaming = $state(false);

  get activeSession() {
    return this.sessions.find(s => s.id === this.activeSessionId) ?? null;
  }

  async loadSessions() {
    try {
      const res = await fetch('/api/ai/sessions');
      if (res.ok) this.sessions = await res.json();
    } catch (e) {
      console.error('Failed to load AI sessions', e);
    }
  }

  async createSession(title = 'New Chat') {
    const res = await fetch('/api/ai/sessions', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ title })
    });
    if (res.ok) {
      const session = await res.json();
      await this.loadSessions();
      this.activeSessionId = session.id;
      this.messages = [];
      return session.id as number;
    }
    return null;
  }

  async selectSession(id: number) {
    this.activeSessionId = id;
    try {
      const res = await fetch(`/api/ai/sessions/${id}`);
      if (res.ok) {
        const data = await res.json();
        this.messages = data.messages ?? [];
      }
    } catch (e) {
      console.error('Failed to load session messages', e);
    }
  }

  async deleteSession(id: number) {
    await fetch(`/api/ai/sessions/${id}`, { method: 'DELETE' });
    await this.loadSessions();
    if (this.activeSessionId === id) {
      this.activeSessionId = null;
      this.messages = [];
    }
  }

  private abortController: AbortController | null = null;

  async sendMessage(text: string) {
    if (!this.activeSessionId || this.isStreaming || !text.trim()) return;

    // Optimistic user message
    this.messages = [...this.messages, {
      id: -Date.now(), uuid: crypto.randomUUID(), session_id: this.activeSessionId,
      role: 'user', content: text, tool_calls: null, tool_results: null,
      is_streaming: 0, created_at: new Date().toISOString()
    }];
    this.isStreaming = true;
    this.streaming = { text: '', toolCalls: [] };
    this.abortController = new AbortController();

    try {
      const res = await fetch('/api/ai/chat', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ session_id: this.activeSessionId, message: text }),
        signal: this.abortController.signal
      });
      if (!res.ok || !res.body) throw new Error(`HTTP ${res.status}`);

      const reader = res.body.getReader();
      const decoder = new TextDecoder();
      let buf = '';

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        buf += decoder.decode(value, { stream: true });
        const parts = buf.split('\n\n');
        buf = parts.pop() ?? '';
        for (const part of parts) {
          const lines = part.split('\n');
          let ev = '', data = '';
          for (const l of lines) {
            if (l.startsWith('event: ')) ev = l.slice(7).trim();
            if (l.startsWith('data: ')) data = l.slice(6).trim();
          }
          if (!ev || !data) continue;
          try { this.handleEvent(ev, JSON.parse(data)); } catch { /* noop */ }
        }
      }
    } catch (e) {
      if (e instanceof Error && e.name !== 'AbortError') {
        addToast({ message: 'AI error: ' + e.message, type: 'error' });
      }
    } finally {
      this.isStreaming = false;
    }
  }

  private handleEvent(event: string, data: Record<string, unknown>) {
    switch (event) {
      case 'text_delta':
        this.streaming = { ...this.streaming, text: this.streaming.text + (data.delta as string ?? '') };
        break;
      case 'tool_start':
        this.streaming = {
          ...this.streaming,
          toolCalls: [...this.streaming.toolCalls, { id: data.id as string, name: data.name as string }]
        };
        break;
      case 'tool_result': {
        const updated = this.streaming.toolCalls.map(tc =>
          tc.id === data.tool_use_id ? { ...tc, result: data.result as string, is_error: data.is_error as boolean } : tc
        );
        this.streaming = { ...this.streaming, toolCalls: updated };
        break;
      }
      case 'done':
        if (this.activeSessionId) {
          this.selectSession(this.activeSessionId).then(() => this.loadSessions());
        }
        this.streaming = { text: '', toolCalls: [] };
        break;
      case 'error':
        addToast({ message: (data.message as string) || 'AI error occurred', type: 'error' });
        this.streaming = { text: '', toolCalls: [] };
        this.isStreaming = false;
        break;
    }
  }

  stopStreaming() {
    this.abortController?.abort();
    this.isStreaming = false;
    this.streaming = { text: '', toolCalls: [] };
  }
}

export const aiChat = new AiChatStore();
