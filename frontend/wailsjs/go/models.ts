export namespace repository {
	
	export class BusinessProfile {
	    name: string;
	    email: string;
	    phone: string;
	    address: string;
	    logo: string;
	    metadata: string;
	    defaultCurrency: string;
	
	    static createFrom(source: any = {}) {
	        return new BusinessProfile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.email = source["email"];
	        this.phone = source["phone"];
	        this.address = source["address"];
	        this.logo = source["logo"];
	        this.metadata = source["metadata"];
	        this.defaultCurrency = source["defaultCurrency"];
	    }
	}
	export class BusinessProfileInput {
	    name: string;
	    email: string;
	    phone: string;
	    address: string;
	    logo: string;
	    metadata: string;
	    defaultCurrency: string;
	
	    static createFrom(source: any = {}) {
	        return new BusinessProfileInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.email = source["email"];
	        this.phone = source["phone"];
	        this.address = source["address"];
	        this.logo = source["logo"];
	        this.metadata = source["metadata"];
	        this.defaultCurrency = source["defaultCurrency"];
	    }
	}

}

