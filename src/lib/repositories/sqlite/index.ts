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
import { SqliteTransactionFactory } from './SqliteTransactionFactory.js';
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

// Shared infrastructure singletons
const auditRepo = new SqliteAuditRepository();
const txFactory = new SqliteTransactionFactory();

export const repositories = {
	invoices: new SqliteInvoiceRepository(auditRepo, txFactory.create()) as InvoiceRepository,
	estimates: new SqliteEstimateRepository(auditRepo, txFactory.create()) as EstimateRepository,
	clients: new SqliteClientRepository(auditRepo, txFactory.create()) as ClientRepository,
	payers: new SqlitePayerRepository(auditRepo, txFactory.create()) as PayerRepository,
	catalog: new SqliteCatalogRepository(auditRepo, txFactory.create()) as CatalogRepository,
	rateTiers: new SqliteRateTierRepository() as RateTierRepository,
	businessProfile: new SqliteBusinessProfileRepository() as BusinessProfileRepository,
	dashboard: new SqliteDashboardRepository() as DashboardRepository,
	audit: auditRepo as AuditRepository,
	columnMappings: new SqliteColumnMappingsRepository() as ColumnMappingsRepository
};

export type Repositories = typeof repositories;
