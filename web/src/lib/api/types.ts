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

export interface RateTier {
	id: number;
	uuid: string;
	name: string;
	description: string;
	sortOrder: number;
	createdAt: string;
	updatedAt: string;
}

export interface RateTierInput {
	name: string;
	description: string;
	sortOrder: number;
}

export interface Payer {
	id: number;
	uuid: string;
	name: string;
	email: string;
	phone: string;
	address: string;
	metadata: string;
	createdAt: string;
	updatedAt: string;
}

export interface PayerInput {
	name: string;
	email: string;
	phone: string;
	address: string;
	metadata: string;
}

export interface TaxRate {
	id: number;
	uuid: string;
	name: string;
	rate: number;
	isDefault: boolean;
	createdAt: string;
	updatedAt: string;
}

export interface TaxRateInput {
	name: string;
	rate: number;
	isDefault: boolean;
}

export interface Client {
	id: number;
	uuid: string;
	name: string;
	email: string;
	phone: string;
	address: string;
	pricingTierId: number | null;
	pricingTierName: string;
	metadata: string;
	payerId: number | null;
	payerName: string;
	createdAt: string;
	updatedAt: string;
}

export interface ClientInput {
	name: string;
	email: string;
	phone: string;
	address: string;
	pricingTierId: number | null;
	metadata: string;
	payerId: number | null;
}

export interface CatalogItem {
	id: number;
	uuid: string;
	name: string;
	rate: number;
	unit: string;
	category: string;
	sku: string;
	metadata: string;
	createdAt: string;
	updatedAt: string;
}

export interface CatalogItemInput {
	name: string;
	rate: number;
	unit: string;
	category: string;
	sku: string;
	metadata: string;
}
