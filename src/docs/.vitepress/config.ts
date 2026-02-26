import { defineConfig } from 'vitepress'

export default defineConfig({
  title: 'Invoice Manager',
  description: 'Documentation for the Invoice Manager PWA',
  base: '/invoices/',
  cleanUrls: true,

  themeConfig: {
    nav: [
      { text: 'Home', link: '/' },
      { text: 'Getting Started', link: '/getting-started' },
      { text: 'Guides', link: '/guides/invoices' },
      { text: 'Open App', link: '/console/', target: '_self' }
    ],

    sidebar: [
      {
        text: 'Introduction',
        items: [
          { text: 'Overview', link: '/' },
          { text: 'Getting Started', link: '/getting-started' },
          { text: 'Features', link: '/features' },
          { text: 'Architecture', link: '/architecture' }
        ]
      },
      {
        text: 'Guides',
        items: [
          { text: 'Invoices', link: '/guides/invoices' },
          { text: 'Estimates', link: '/guides/estimates' },
          { text: 'Clients', link: '/guides/clients' },
          { text: 'Catalog', link: '/guides/catalog' },
          { text: 'Import & Export', link: '/guides/import-export' },
          { text: 'PDF Generation', link: '/guides/pdf-generation' },
          { text: 'Settings', link: '/guides/settings' }
        ]
      }
    ],

    socialLinks: [
      { icon: 'github', link: 'https://github.com/dknathalage/invoices' }
    ],

    footer: {
      message: 'Invoice Manager — Local-first invoice management'
    },

    search: {
      provider: 'local'
    }
  }
})
