export namespace agent {
	
	export class Message {
	    role: string;
	    content: string;
	
	    static createFrom(source: any = {}) {
	        return new Message(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.role = source["role"];
	        this.content = source["content"];
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

