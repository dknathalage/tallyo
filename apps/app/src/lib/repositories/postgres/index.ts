import { PgInvoiceRepository } from './PgInvoiceRepository.js';
import { PgEstimateRepository } from './PgEstimateRepository.js';
import { PgClientRepository } from './PgClientRepository.js';
import { PgPayerRepository } from './PgPayerRepository.js';
import { PgCatalogRepository } from './PgCatalogRepository.js';
import { PgRateTierRepository } from './PgRateTierRepository.js';
import { PgTaxRateRepository } from './PgTaxRateRepository.js';
import { PgPaymentRepository } from './PgPaymentRepository.js';
import { PgBusinessProfileRepository } from './PgBusinessProfileRepository.js';
import { PgDashboardRepository } from './PgDashboardRepository.js';
import { PgAuditRepository } from './PgAuditRepository.js';
import { PgColumnMappingsRepository } from './PgColumnMappingsRepository.js';
import { PgRecurringTemplateRepository } from './PgRecurringTemplateRepository.js';
import { PgAiChatRepository } from './PgAiChatRepository.js';
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

const auditRepo = new PgAuditRepository();

export const repositories = {
	invoices: new PgInvoiceRepository(auditRepo) as InvoiceRepository,
	estimates: new PgEstimateRepository(auditRepo) as EstimateRepository,
	clients: new PgClientRepository(auditRepo) as ClientRepository,
	payers: new PgPayerRepository(auditRepo) as PayerRepository,
	catalog: new PgCatalogRepository(auditRepo) as CatalogRepository,
	rateTiers: new PgRateTierRepository() as RateTierRepository,
	taxRates: new PgTaxRateRepository() as TaxRateRepository,
	payments: new PgPaymentRepository() as PaymentRepository,
	businessProfile: new PgBusinessProfileRepository() as BusinessProfileRepository,
	dashboard: new PgDashboardRepository() as DashboardRepository,
	audit: auditRepo as AuditRepository,
	columnMappings: new PgColumnMappingsRepository() as ColumnMappingsRepository,
	recurringTemplates: new PgRecurringTemplateRepository() as RecurringTemplateRepository,
	aiChat: new PgAiChatRepository() as AiChatRepository
};

export type Repositories = typeof repositories;
