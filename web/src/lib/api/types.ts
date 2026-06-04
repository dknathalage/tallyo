export type Role = 'owner' | 'admin' | 'member' | string;

export interface User {
	id: number;
	uuid: string;
	email: string;
	role: Role;
	lastLoginAt: string | null;
}

export interface SetupStatus {
	ownerExists: boolean;
}

export interface InviteInfo {
	email: string;
	role: Role;
}

export interface InviteCreated {
	token: string;
	acceptUrl: string;
}
