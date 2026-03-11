import { SqliteInvoiceRepository } from './SqliteInvoiceRepository.js';
import { SqliteEstimateRepository } from './SqliteEstimateRepository.js';
import { SqliteClientRepository } from './SqliteClientRepository.js';
import { SqlitePayerRepository } from './SqlitePayerRepository.js';
import { SqliteCatalogRepository } from './SqliteCatalogRepository.js';
import { SqliteRateTierRepository } from './SqliteRateTierRepository.js';
import { SqliteBusinessProfileRepository } from './SqliteBusinessProfileRepository.js';
import { SqliteDashboardRepository } from './SqliteDashboardRepository.js';
import { SqliteAuditRepository } from './SqliteAuditRepository.js';
import { SqliteColumnMappingsRepository } from './SqliteColumnMappingsRepository.js';
import type {
	InvoiceRepository,
	EstimateRepository,
	ClientRepository,
	PayerRepository,
	CatalogRepository,
	RateTierRepository,
	BusinessProfileRepository,
	DashboardRepository,
	AuditRepository,
	ColumnMappingsRepository
} from '../interfaces/index.js';

export const repositories = {
	invoices: new SqliteInvoiceRepository() as InvoiceRepository,
	estimates: new SqliteEstimateRepository() as EstimateRepository,
	clients: new SqliteClientRepository() as ClientRepository,
	payers: new SqlitePayerRepository() as PayerRepository,
	catalog: new SqliteCatalogRepository() as CatalogRepository,
	rateTiers: new SqliteRateTierRepository() as RateTierRepository,
	businessProfile: new SqliteBusinessProfileRepository() as BusinessProfileRepository,
	dashboard: new SqliteDashboardRepository() as DashboardRepository,
	audit: new SqliteAuditRepository() as AuditRepository,
	columnMappings: new SqliteColumnMappingsRepository() as ColumnMappingsRepository
};

export type Repositories = typeof repositories;
