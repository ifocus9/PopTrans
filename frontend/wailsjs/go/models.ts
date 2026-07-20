export namespace backend {
	
	export class Health {
	    status: string;
	    translator_ready: boolean;
	    translator_status: string;
	    ocr_loaded: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Health(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.translator_ready = source["translator_ready"];
	        this.translator_status = source["translator_status"];
	        this.ocr_loaded = source["ocr_loaded"];
	    }
	}

}

export namespace config {
	
	export class Config {
	    hotkey: string;
	    hotkey_display: string;
	    ocr_enabled: boolean;
	    ocr_hotkey: string;
	    ocr_hotkey_display: string;
	    logging_enabled: boolean;
	    server_port: number;
	    theme: string;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hotkey = source["hotkey"];
	        this.hotkey_display = source["hotkey_display"];
	        this.ocr_enabled = source["ocr_enabled"];
	        this.ocr_hotkey = source["ocr_hotkey"];
	        this.ocr_hotkey_display = source["ocr_hotkey_display"];
	        this.logging_enabled = source["logging_enabled"];
	        this.server_port = source["server_port"];
	        this.theme = source["theme"];
	    }
	}

}

export namespace wailsui {
	
	export class ResultView {
	    source: string;
	    result: string;
	    error: string;
	    loading: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ResultView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.source = source["source"];
	        this.result = source["result"];
	        this.error = source["error"];
	        this.loading = source["loading"];
	    }
	}
	export class TranslateResult {
	    source: string;
	    result: string;
	
	    static createFrom(source: any = {}) {
	        return new TranslateResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.source = source["source"];
	        this.result = source["result"];
	    }
	}
	export class UIState {
	    config: config.Config;
	    health: backend.Health;
	    mode: string;
	    result: ResultView;
	
	    static createFrom(source: any = {}) {
	        return new UIState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.config = this.convertValues(source["config"], config.Config);
	        this.health = this.convertValues(source["health"], backend.Health);
	        this.mode = source["mode"];
	        this.result = this.convertValues(source["result"], ResultView);
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

