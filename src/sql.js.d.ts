declare module 'sql.js' {
	interface Database {
		run(sql: string, params?: any[]): Database;
		exec(sql: string, params?: any[]): QueryExecResult[];
		prepare(sql: string): Statement;
		export(): Uint8Array;
		close(): void;
	}

	interface Statement {
		bind(params?: any[]): boolean;
		step(): boolean;
		getAsObject(): Record<string, any>;
		free(): void;
	}

	interface QueryExecResult {
		columns: string[];
		values: any[][];
	}

	interface SqlJsStatic {
		Database: new (data?: ArrayLike<number>) => Database;
	}

	export type { Database, Statement, QueryExecResult, SqlJsStatic };

	export default function initSqlJs(config?: {
		locateFile?: (file: string) => string;
	}): Promise<SqlJsStatic>;
}
