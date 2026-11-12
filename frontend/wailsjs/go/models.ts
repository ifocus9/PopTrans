export namespace backend {
	
	export class Health {
	    status: string;
	    translator_ready: boolean;
	    translator_status: string;
	    ocr_loaded: boolean;
	    device_info?: Record<string, any>;
	
	    static createFrom(source: any = {}) {
	        return new Health(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.translator_ready = source["translator_ready"];
	        this.translator_status = source["translator_status"];
	        this.ocr_loaded = source["ocr_loaded"];
	        this.device_info = source["device_info"];
	    }
	}

}

export namespace config {
	
	export class ModelEntry {
	    id: string;
	    name: string;
	    source: string;
	    repo: string;
	    file: string;
	    size?: number;
	    dir?: string;
	    builtin?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ModelEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.source = source["source"];
	        this.repo = source["repo"];
	        this.file = source["file"];
	        this.size = source["size"];
	        this.dir = source["dir"];
	        this.builtin = source["builtin"];
	    }
	}
	
	export class GlossaryEntry {
	    term: string;
	    translation: string;
	    source: string;
	    note?: string;
	
	    static createFrom(source: any = {}) {
	        return new GlossaryEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.term = source["term"];
	        this.translation = source["translation"];
	        this.source = source["source"];
	        this.note = source["note"];
	    }
	}
	export class Config {
	    hotkey: string;
	    hotkey_display: string;
	    ocr_enabled: boolean;
	    ocr_hotkey: string;
	    ocr_hotkey_display: string;
	    live_enabled: boolean;
	    live_hotkey: string;
	    live_hotkey_display: string;
	    logging_enabled: boolean;
	    server_port: number;
	    theme: string;
	    acceleration_device: string;
	    ui_idle_minutes: number;
	    ai_idle_minutes: number;
	    ai_keep_alive: boolean;
	    ai_threads: number;
	    launch_at_login: boolean;
	    model_download_source: string;
	    model_download_base_url: string;
	    models: ModelEntry[];
	    active_model_id: string;
	    source_language: string;
	    target_language: string;
	    overlay_font_family: string;
	    overlay_font_weight: number;
	    glossary: GlossaryEntry[];
	
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
	        this.live_enabled = source["live_enabled"];
	        this.live_hotkey = source["live_hotkey"];
	        this.live_hotkey_display = source["live_hotkey_display"];
	        this.logging_enabled = source["logging_enabled"];
	        this.server_port = source["server_port"];
	        this.theme = source["theme"];
	        this.acceleration_device = source["acceleration_device"];
	        this.ui_idle_minutes = source["ui_idle_minutes"];
	        this.ai_idle_minutes = source["ai_idle_minutes"];
	        this.ai_keep_alive = source["ai_keep_alive"];
	        this.ai_threads = source["ai_threads"];
	        this.launch_at_login = source["launch_at_login"];
	        this.model_download_source = source["model_download_source"];
	        this.model_download_base_url = source["model_download_base_url"];
	        this.models = this.convertValues(source["models"], ModelEntry);
	        this.active_model_id = source["active_model_id"];
	        this.source_language = source["source_language"];
	        this.target_language = source["target_language"];
	        this.overlay_font_family = source["overlay_font_family"];
	        this.overlay_font_weight = source["overlay_font_weight"];
	        this.glossary = this.convertValues(source["glossary"], GlossaryEntry);
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

export namespace uidaemon {
	
	export class GlossaryHit {
	    term: string;
	    translation: string;
	    source: string;
	    note?: string;
	
	    static createFrom(source: any = {}) {
	        return new GlossaryHit(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.term = source["term"];
	        this.translation = source["translation"];
	        this.source = source["source"];
	        this.note = source["note"];
	    }
	}
	export class OverlayStyle {
	    font_family: string;
	    font_weight: number;
	
	    static createFrom(source: any = {}) {
	        return new OverlayStyle(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.font_family = source["font_family"];
	        this.font_weight = source["font_weight"];
	    }
	}
	export class TranslatedBlock {
	    text: string;
	    translation: string;
	    x: number;
	    y: number;
	    w: number;
	    h: number;
	    score: number;
	    glossary_hits?: GlossaryHit[];
	    angle?: number;
	    cx?: number;
	    cy?: number;
	    line_width?: number;
	    line_height?: number;
	    source_h?: number;
	    text_color?: string;
	    background_image?: string;
	
	    static createFrom(source: any = {}) {
	        return new TranslatedBlock(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.text = source["text"];
	        this.translation = source["translation"];
	        this.x = source["x"];
	        this.y = source["y"];
	        this.w = source["w"];
	        this.h = source["h"];
	        this.score = source["score"];
	        this.glossary_hits = this.convertValues(source["glossary_hits"], GlossaryHit);
	        this.angle = source["angle"];
	        this.cx = source["cx"];
	        this.cy = source["cy"];
	        this.line_width = source["line_width"];
	        this.line_height = source["line_height"];
	        this.source_h = source["source_h"];
	        this.text_color = source["text_color"];
	        this.background_image = source["background_image"];
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
	export class ScreenRect {
	    x: number;
	    y: number;
	    width: number;
	    height: number;
	
	    static createFrom(source: any = {}) {
	        return new ScreenRect(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.x = source["x"];
	        this.y = source["y"];
	        this.width = source["width"];
	        this.height = source["height"];
	    }
	}
	export class LivePayload {
	    anchor: ScreenRect;
	    window_rect: ScreenRect;
	    bar_side: string;
	    image_data_url?: string;
	    image_width: number;
	    image_height: number;
	    blocks: TranslatedBlock[];
	    source: string;
	    translation: string;
	    status: string;
	    error: string;
	    style: OverlayStyle;
	    version: number;
	    latency_ms: number;
	
	    static createFrom(source: any = {}) {
	        return new LivePayload(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.anchor = this.convertValues(source["anchor"], ScreenRect);
	        this.window_rect = this.convertValues(source["window_rect"], ScreenRect);
	        this.bar_side = source["bar_side"];
	        this.image_data_url = source["image_data_url"];
	        this.image_width = source["image_width"];
	        this.image_height = source["image_height"];
	        this.blocks = this.convertValues(source["blocks"], TranslatedBlock);
	        this.source = source["source"];
	        this.translation = source["translation"];
	        this.status = source["status"];
	        this.error = source["error"];
	        this.style = this.convertValues(source["style"], OverlayStyle);
	        this.version = source["version"];
	        this.latency_ms = source["latency_ms"];
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
	export class OverlayPayload {
	    rect: ScreenRect;
	    window_rect: ScreenRect;
	    bar_side: string;
	    image_width: number;
	    image_height: number;
	    image_data_url?: string;
	    blocks: TranslatedBlock[];
	    source: string;
	    translation: string;
	    loading: boolean;
	    error: string;
	    style: OverlayStyle;
	    latency_ms: number;
	
	    static createFrom(source: any = {}) {
	        return new OverlayPayload(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rect = this.convertValues(source["rect"], ScreenRect);
	        this.window_rect = this.convertValues(source["window_rect"], ScreenRect);
	        this.bar_side = source["bar_side"];
	        this.image_width = source["image_width"];
	        this.image_height = source["image_height"];
	        this.image_data_url = source["image_data_url"];
	        this.blocks = this.convertValues(source["blocks"], TranslatedBlock);
	        this.source = source["source"];
	        this.translation = source["translation"];
	        this.loading = source["loading"];
	        this.error = source["error"];
	        this.style = this.convertValues(source["style"], OverlayStyle);
	        this.latency_ms = source["latency_ms"];
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
	
	export class ResultPayload {
	    source: string;
	    result: string;
	    error: string;
	    loading: boolean;
	    latency_ms: number;
	
	    static createFrom(source: any = {}) {
	        return new ResultPayload(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.source = source["source"];
	        this.result = source["result"];
	        this.error = source["error"];
	        this.loading = source["loading"];
	        this.latency_ms = source["latency_ms"];
	    }
	}
	
	export class Status {
	    ready: boolean;
	    mode: string;
	    visible: boolean;
	    settings_open: boolean;
	    hotkeys_paused: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ready = source["ready"];
	        this.mode = source["mode"];
	        this.visible = source["visible"];
	        this.settings_open = source["settings_open"];
	        this.hotkeys_paused = source["hotkeys_paused"];
	    }
	}

}

export namespace wailsui {
	
	export class ModelSearchItem {
	    id: string;
	    name: string;
	    source: string;
	    downloads: number;
	    description: string;
	    size_bytes: number;
	    parameters: string;
	
	    static createFrom(source: any = {}) {
	        return new ModelSearchItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.source = source["source"];
	        this.downloads = source["downloads"];
	        this.description = source["description"];
	        this.size_bytes = source["size_bytes"];
	        this.parameters = source["parameters"];
	    }
	}
	export class ModelFileItem {
	    name: string;
	    size: number;
	
	    static createFrom(source: any = {}) {
	        return new ModelFileItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.size = source["size"];
	    }
	}
	export class ModelStatus {
	    id: string;
	    installed: boolean;
	    size: number;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new ModelStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.installed = source["installed"];
	        this.size = source["size"];
	        this.path = source["path"];
	    }
	}
	export class ModelDownloadEvent {
	    status: string;
	    source: string;
	    repo: string;
	    file: string;
	    name: string;
	    downloaded: number;
	    total: number;
	    percent: number;
	    speed: number;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new ModelDownloadEvent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.source = source["source"];
	        this.repo = source["repo"];
	        this.file = source["file"];
	        this.name = source["name"];
	        this.downloaded = source["downloaded"];
	        this.total = source["total"];
	        this.percent = source["percent"];
	        this.speed = source["speed"];
	        this.error = source["error"];
	    }
	}
	
	export class ResultView {
	    source: string;
	    result: string;
	    error: string;
	    loading: boolean;
	    latency_ms: number;
	
	    static createFrom(source: any = {}) {
	        return new ResultView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.source = source["source"];
	        this.result = source["result"];
	        this.error = source["error"];
	        this.loading = source["loading"];
	        this.latency_ms = source["latency_ms"];
	    }
	}
	export class TranslateResult {
	    source: string;
	    result: string;
	    latency_ms: number;
	
	    static createFrom(source: any = {}) {
	        return new TranslateResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.source = source["source"];
	        this.result = source["result"];
	        this.latency_ms = source["latency_ms"];
	    }
	}
	export class UIState {
	    config: config.Config;
	    health: backend.Health;
	    mode: string;
	    result: ResultView;
	    startup_loading: boolean;
	    startup_status: string;
	    startup_error: string;
	    settings_open: boolean;
	    visible: boolean;
	
	    static createFrom(source: any = {}) {
	        return new UIState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.config = this.convertValues(source["config"], config.Config);
	        this.health = this.convertValues(source["health"], backend.Health);
	        this.mode = source["mode"];
	        this.result = this.convertValues(source["result"], ResultView);
	        this.startup_loading = source["startup_loading"];
	        this.startup_status = source["startup_status"];
	        this.startup_error = source["startup_error"];
	        this.settings_open = source["settings_open"];
	        this.visible = source["visible"];
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

