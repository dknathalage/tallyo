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
import { SqliteAiChatRepository } from './SqliteAiChatRepository.js';
import type {
	InvoiceRepository,
	EstimateRepository,
	ClientRepository,
	PayerRepository,
	CatalogRepository,
	RateTierRepository,
	TaxRateRepository,
	PaymentRepository,
	BusinessProfileRepository,
	DashboardRepository,
	AuditRepository,
	ColumnMappingsRepository,
	RecurringTemplateRepository
} from '../interfaces/index.js';
import type { AiChatRepository } from '../interfaces/AiChatRepository.js';

const auditRepo = new SqliteAuditRepository();

export const repositories = {
	invoices: new SqliteInvoiceRepository(auditRepo) as InvoiceRepository,
	estimates: new SqliteEstimateRepository(auditRepo) as EstimateRepository,
	clients: new SqliteClientRepository(auditRepo) as ClientRepository,
	payers: new SqlitePayerRepository(auditRepo) as PayerRepository,
	catalog: new SqliteCatalogRepository(auditRepo) as CatalogRepository,
	rateTiers: new SqliteRateTierRepository() as RateTierRepository,
	taxRates: new SqliteTaxRateRepository() as TaxRateRepository,
	payments: new SqlitePaymentRepository() as PaymentRepository,
	businessProfile: new SqliteBusinessProfileRepository() as BusinessProfileRepository,
	dashboard: new SqliteDashboardRepository() as DashboardRepository,
	audit: auditRepo as AuditRepository,
	columnMappings: new SqliteColumnMappingsRepository() as ColumnMappingsRepository,
	recurringTemplates: new SqliteRecurringTemplateRepository() as RecurringTemplateRepository,
	aiChat: new SqliteAiChatRepository() as AiChatRepository
};

export type Repositories = typeof repositories;
