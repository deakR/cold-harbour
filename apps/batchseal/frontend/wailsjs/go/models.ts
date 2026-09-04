export namespace backend {
	
	export class Audit {
	    id: string;
	    compartmentId: string;
	    taskType: string;
	    finalState: string;
	    checksum: string;
	    durationMs: number;
	    completedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new Audit(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.compartmentId = source["compartmentId"];
	        this.taskType = source["taskType"];
	        this.finalState = source["finalState"];
	        this.checksum = source["checksum"];
	        this.durationMs = source["durationMs"];
	        this.completedAt = source["completedAt"];
	    }
	}
	export class Compartment {
	    compartmentId: string;
	    context: string;
	    ownerId: string;
	    taskType: string;
	    state: string;
	    progress: number;
	    details: string;
	
	    static createFrom(source: any = {}) {
	        return new Compartment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.compartmentId = source["compartmentId"];
	        this.context = source["context"];
	        this.ownerId = source["ownerId"];
	        this.taskType = source["taskType"];
	        this.state = source["state"];
	        this.progress = source["progress"];
	        this.details = source["details"];
	    }
	}
	export class Config {
	    baseURL: string;
	    apiKey: string;
	    clearance: string;
	    ownerID: string;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.baseURL = source["baseURL"];
	        this.apiKey = source["apiKey"];
	        this.clearance = source["clearance"];
	        this.ownerID = source["ownerID"];
	    }
	}
	export class Receipt {
	    compartmentId: string;
	    taskType: string;
	    output: Record<string, any>;
	    checksum: string;
	    verified: boolean;
	    remainingTtlSeconds: number;
	    archivedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new Receipt(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.compartmentId = source["compartmentId"];
	        this.taskType = source["taskType"];
	        this.output = source["output"];
	        this.checksum = source["checksum"];
	        this.verified = source["verified"];
	        this.remainingTtlSeconds = source["remainingTtlSeconds"];
	        this.archivedAt = source["archivedAt"];
	    }
	}
	export class Worker {
	    workerId: string;
	    status: string;
	    healthy: boolean;
	    activeCompartmentId: string;
	    secondsSinceLastHeartbeat: number;
	
	    static createFrom(source: any = {}) {
	        return new Worker(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.workerId = source["workerId"];
	        this.status = source["status"];
	        this.healthy = source["healthy"];
	        this.activeCompartmentId = source["activeCompartmentId"];
	        this.secondsSinceLastHeartbeat = source["secondsSinceLastHeartbeat"];
	    }
	}

}

