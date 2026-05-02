import { SqliteInvoiceRepository } from './SqliteInvoiceRepository.js';
import { SqliteEstimateRepository } from './SqliteEstimateRepository.js';
import { SqliteClientRepository } from './SqliteClientRepository.js';
import { SqlitePayerRepository } from './SqlitePayerRepository.js';
import { SqliteCatalogRepository } from './SqliteCatalogRepository.js';
import { SqliteRateTierRepository } from './SqliteRateTierRepository.js';
import { SqliteTaxRateRepository } from './SqliteTaxRateRepository.js';
import { SqlitePaymentRepository } from './SqlitePaymentRepository.js';
import { SqliteBusinessProfileRepository } from './SqliteBusinessProfileRepository.js';
import { SqliteDashboardRepository } from './SqliteDashboardRepository.js';
import { SqliteAuditRepository } from './SqliteAuditRepository.js';
import { SqliteColumnMappingsRepository } from './SqliteColumnMappingsRepository.js';
import { SqliteRecurringTemplateRepository } from './SqliteRecurringTemplateRepository.js';
import type {
	InvoiceRepository,
	EstimateRepository,
	ClientRepository,
	PayerRepository,
	CatalogRepository
} from '../interfaces/index.js';

const auditRepo = new SqliteAuditRepository();

export const repositories = {
	invoices: new SqliteInvoiceRepository(auditRepo) as InvoiceRepository,
	estimates: new SqliteEstimateRepository(auditRepo) as EstimateRepository,
	clients: new SqliteClientRepository(auditRepo) as ClientRepository,
	payers: new SqlitePayerRepository(auditRepo) as PayerRepository,
	catalog: new SqliteCatalogRepository(auditRepo) as CatalogRepository,
	rateTiers: new SqliteRateTierRepository(),
	taxRates: new SqliteTaxRateRepository(),
	payments: new SqlitePaymentRepository(),
	businessProfile: new SqliteBusinessProfileRepository(),
	dashboard: new SqliteDashboardRepository(),
	audit: auditRepo,
	columnMappings: new SqliteColumnMappingsRepository(),
	recurringTemplates: new SqliteRecurringTemplateRepository()
};

export type Repositories = typeof repositories;
