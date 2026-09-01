export namespace agent {
	
	export class GameContextResponse {
	    available: boolean;
	    game: string;
	    name: string;
	    phase: string;
	    detected_at?: string;
	    window_title?: string;
	    window_class?: string;
	    game_data_type?: string;
	
	    static createFrom(source: any = {}) {
	        return new GameContextResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.game = source["game"];
	        this.name = source["name"];
	        this.phase = source["phase"];
	        this.detected_at = source["detected_at"];
	        this.window_title = source["window_title"];
	        this.window_class = source["window_class"];
	        this.game_data_type = source["game_data_type"];
	    }
	}
	export class Message {
	    role: string;
	    content: string;
	    screenshot_attached?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Message(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.role = source["role"];
	        this.content = source["content"];
	        this.screenshot_attached = source["screenshot_attached"];
	    }
	}
	export class WakeWordConfigResponse {
	    enabled: boolean;
	    model_path: string;
	    threshold: number;
	    cooldown_ms: number;
	    vad_enabled: boolean;
	    onnx_runtime_path: string;
	
	    static createFrom(source: any = {}) {
	        return new WakeWordConfigResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.model_path = source["model_path"];
	        this.threshold = source["threshold"];
	        this.cooldown_ms = source["cooldown_ms"];
	        this.vad_enabled = source["vad_enabled"];
	        this.onnx_runtime_path = source["onnx_runtime_path"];
	    }
	}

}

