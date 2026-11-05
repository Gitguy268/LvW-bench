export namespace main {
	
	export class Info {
	    version: string;
	    bench: string;
	    llama_server: string;
	    uv: string;
	    results_dir: string;
	    os: string;
	    presets: presets.Preset[];
	    configs: tasks.Config[];
	
	    static createFrom(source: any = {}) {
	        return new Info(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.bench = source["bench"];
	        this.llama_server = source["llama_server"];
	        this.uv = source["uv"];
	        this.results_dir = source["results_dir"];
	        this.os = source["os"];
	        this.presets = this.convertValues(source["presets"], presets.Preset);
	        this.configs = this.convertValues(source["configs"], tasks.Config);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class RunRequest {
	    model: string;
	    tracks: string[];
	    profile: string;
	    quant: string;
	    subfolder: string;
	    backend: string;
	    server_model: string;
	    mode: string;
	    trust_remote_code: boolean;
	    cpu_only: boolean;
	    configs: string[];
	
	    static createFrom(source: any = {}) {
	        return new RunRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.model = source["model"];
	        this.tracks = source["tracks"];
	        this.profile = source["profile"];
	        this.quant = source["quant"];
	        this.subfolder = source["subfolder"];
	        this.backend = source["backend"];
	        this.server_model = source["server_model"];
	        this.mode = source["mode"];
	        this.trust_remote_code = source["trust_remote_code"];
	        this.cpu_only = source["cpu_only"];
	        this.configs = source["configs"];
	    }
	}
	export class RunResponse {
	    result?: results.Result;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new RunResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.result = this.convertValues(source["result"], results.Result);
	        this.path = source["path"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace presets {
	
	export class Preset {
	    name: string;
	    input: string;
	    kind: string;
	    tracks: string[];
	    args?: string[];
	    comment: string;
	    mode?: string;
	
	    static createFrom(source: any = {}) {
	        return new Preset(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.input = source["input"];
	        this.kind = source["kind"];
	        this.tracks = source["tracks"];
	        this.args = source["args"];
	        this.comment = source["comment"];
	        this.mode = source["mode"];
	    }
	}

}

export namespace results {
	
	export class ConfigScore {
	    n: number;
	    accuracy: number;
	    kl_from_gold?: number;
	    brier?: number;
	    ece?: number;
	    coverage?: number;
	
	    static createFrom(source: any = {}) {
	        return new ConfigScore(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.n = source["n"];
	        this.accuracy = source["accuracy"];
	        this.kl_from_gold = source["kl_from_gold"];
	        this.brier = source["brier"];
	        this.ece = source["ece"];
	        this.coverage = source["coverage"];
	    }
	}
	export class Model {
	    label: string;
	    input: string;
	    format: string;
	    repo?: string;
	    sha?: string;
	    file?: string;
	    quant?: string;
	    size_bytes?: number;
	    file_sha256?: string;
	    backend: string;
	    params?: string;
	
	    static createFrom(source: any = {}) {
	        return new Model(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.label = source["label"];
	        this.input = source["input"];
	        this.format = source["format"];
	        this.repo = source["repo"];
	        this.sha = source["sha"];
	        this.file = source["file"];
	        this.quant = source["quant"];
	        this.size_bytes = source["size_bytes"];
	        this.file_sha256 = source["file_sha256"];
	        this.backend = source["backend"];
	        this.params = source["params"];
	    }
	}
	export class Track {
	    accuracy: number;
	    ci_low: number;
	    ci_high: number;
	    kl_from_gold?: number;
	    brier?: number;
	    ece?: number;
	    n: number;
	    coverage?: number;
	    adapter?: string;
	    mean_output_tokens?: number;
	    tokens_per_second?: number;
	    p50_ms: number;
	    wall_seconds: number;
	    truncation_rate?: number;
	    native_thinking?: boolean;
	    errors?: number;
	    per_config: Record<string, ConfigScore>;
	
	    static createFrom(source: any = {}) {
	        return new Track(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.accuracy = source["accuracy"];
	        this.ci_low = source["ci_low"];
	        this.ci_high = source["ci_high"];
	        this.kl_from_gold = source["kl_from_gold"];
	        this.brier = source["brier"];
	        this.ece = source["ece"];
	        this.n = source["n"];
	        this.coverage = source["coverage"];
	        this.adapter = source["adapter"];
	        this.mean_output_tokens = source["mean_output_tokens"];
	        this.tokens_per_second = source["tokens_per_second"];
	        this.p50_ms = source["p50_ms"];
	        this.wall_seconds = source["wall_seconds"];
	        this.truncation_rate = source["truncation_rate"];
	        this.native_thinking = source["native_thinking"];
	        this.errors = source["errors"];
	        this.per_config = this.convertValues(source["per_config"], ConfigScore, true);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Result {
	    bench: string;
	    bench_version: string;
	    // Go type: time
	    created: any;
	    model: Model;
	    mode: string;
	    profile: string;
	    seed: number;
	    tracks: Record<string, Track>;
	    system: Record<string, string>;
	    notes?: string;
	
	    static createFrom(source: any = {}) {
	        return new Result(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.bench = source["bench"];
	        this.bench_version = source["bench_version"];
	        this.created = this.convertValues(source["created"], null);
	        this.model = this.convertValues(source["model"], Model);
	        this.mode = source["mode"];
	        this.profile = source["profile"];
	        this.seed = source["seed"];
	        this.tracks = this.convertValues(source["tracks"], Track, true);
	        this.system = source["system"];
	        this.notes = source["notes"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace tasks {
	
	export class Config {
	    name: string;
	    kind: string;
	    description: string;
	    source: string;
	    license: string;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.description = source["description"];
	        this.source = source["source"];
	        this.license = source["license"];
	    }
	}

}

