export namespace main {
	
	export class ChapterDTO {
	    index: number;
	    number: number;
	    title: string;
	    language: string;
	
	    static createFrom(source: any = {}) {
	        return new ChapterDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.index = source["index"];
	        this.number = source["number"];
	        this.title = source["title"];
	        this.language = source["language"];
	    }
	}
	export class JobDTO {
	    title: string;
	    number: number;
	    total: number;
	    done: number;
	    state: string;
	    reason: string;
	    status: string;
	
	    static createFrom(source: any = {}) {
	        return new JobDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.number = source["number"];
	        this.total = source["total"];
	        this.done = source["done"];
	        this.state = source["state"];
	        this.reason = source["reason"];
	        this.status = source["status"];
	    }
	}
	export class LogDTO {
	    level: string;
	    text: string;
	
	    static createFrom(source: any = {}) {
	        return new LogDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.level = source["level"];
	        this.text = source["text"];
	    }
	}
	export class OptionsDTO {
	    outputDir: string;
	    format: string;
	    bundle: boolean;
	    language: string;
	    scanlator: string;
	    convertImages: string;
	    skipPages: string;
	    filenameTemplate: string;
	    browserVisible: boolean;
	
	    static createFrom(source: any = {}) {
	        return new OptionsDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.outputDir = source["outputDir"];
	        this.format = source["format"];
	        this.bundle = source["bundle"];
	        this.language = source["language"];
	        this.scanlator = source["scanlator"];
	        this.convertImages = source["convertImages"];
	        this.skipPages = source["skipPages"];
	        this.filenameTemplate = source["filenameTemplate"];
	        this.browserVisible = source["browserVisible"];
	    }
	}
	export class RunDTO {
	    chapters: number;
	    bundle: boolean;
	
	    static createFrom(source: any = {}) {
	        return new RunDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.chapters = source["chapters"];
	        this.bundle = source["bundle"];
	    }
	}
	export class SummaryDTO {
	    dir: string;
	    cancelled: boolean;
	    saved: number;
	    bundled: boolean;
	    bundle: string;
	    skipped: number;
	    failures: string[];
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new SummaryDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dir = source["dir"];
	        this.cancelled = source["cancelled"];
	        this.saved = source["saved"];
	        this.bundled = source["bundled"];
	        this.bundle = source["bundle"];
	        this.skipped = source["skipped"];
	        this.failures = source["failures"];
	        this.error = source["error"];
	    }
	}
	export class State {
	    url: string;
	    loading: boolean;
	    loaded: boolean;
	    loadError: string;
	    title: string;
	    chapterCount: number;
	    hasDuplicates: boolean;
	    stale: boolean;
	    loadedLanguage: string;
	    loadedScanlator: string;
	    selectedCount: number;
	    running: boolean;
	    cancelling: boolean;
	    run: RunDTO;
	    jobs: JobDTO[];
	    summary?: SummaryDTO;
	    options: OptionsDTO;
	    version: string;
	
	    static createFrom(source: any = {}) {
	        return new State(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.loading = source["loading"];
	        this.loaded = source["loaded"];
	        this.loadError = source["loadError"];
	        this.title = source["title"];
	        this.chapterCount = source["chapterCount"];
	        this.hasDuplicates = source["hasDuplicates"];
	        this.stale = source["stale"];
	        this.loadedLanguage = source["loadedLanguage"];
	        this.loadedScanlator = source["loadedScanlator"];
	        this.selectedCount = source["selectedCount"];
	        this.running = source["running"];
	        this.cancelling = source["cancelling"];
	        this.run = this.convertValues(source["run"], RunDTO);
	        this.jobs = this.convertValues(source["jobs"], JobDTO);
	        this.summary = this.convertValues(source["summary"], SummaryDTO);
	        this.options = this.convertValues(source["options"], OptionsDTO);
	        this.version = source["version"];
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

