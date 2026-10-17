export namespace main {
	
	export class ChapterDTO {
	    index: number;
	    number: number;
	    title: string;
	    language: string;
	    group: string;
	    volume: number;
	    locked: boolean;
	    part?: string;
	    wholeTitle?: string;
	    unnumbered?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ChapterDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.index = source["index"];
	        this.number = source["number"];
	        this.title = source["title"];
	        this.language = source["language"];
	        this.group = source["group"];
	        this.volume = source["volume"];
	        this.locked = source["locked"];
	        this.part = source["part"];
	        this.wholeTitle = source["wholeTitle"];
	        this.unnumbered = source["unnumbered"];
	    }
	}
	export class FilterDTO {
	    values: string[];
	    all: boolean;
	    selected: string;
	
	    static createFrom(source: any = {}) {
	        return new FilterDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.values = source["values"];
	        this.all = source["all"];
	        this.selected = source["selected"];
	    }
	}
	export class Message {
	    key: string;
	    args?: Record<string, any>;
	    text: string;
	
	    static createFrom(source: any = {}) {
	        return new Message(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.args = source["args"];
	        this.text = source["text"];
	    }
	}
	export class JobDTO {
	    title: string;
	    titleMsg?: Message;
	    tags: string;
	    number: number;
	    total: number;
	    done: number;
	    state: string;
	    reason: string;
	    reasonMsg?: Message;
	    status: string;
	    statusMsg?: Message;
	    volume: boolean;
	
	    static createFrom(source: any = {}) {
	        return new JobDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.titleMsg = this.convertValues(source["titleMsg"], Message);
	        this.tags = source["tags"];
	        this.number = source["number"];
	        this.total = source["total"];
	        this.done = source["done"];
	        this.state = source["state"];
	        this.reason = source["reason"];
	        this.reasonMsg = this.convertValues(source["reasonMsg"], Message);
	        this.status = source["status"];
	        this.statusMsg = this.convertValues(source["statusMsg"], Message);
	        this.volume = source["volume"];
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
	export class LogDTO {
	    level: string;
	    text: string;
	    msg?: Message;
	
	    static createFrom(source: any = {}) {
	        return new LogDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.level = source["level"];
	        this.text = source["text"];
	        this.msg = this.convertValues(source["msg"], Message);
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
	
	export class OptionsDTO {
	    outputDir: string;
	    format: string;
	    readingDirection: string;
	    packAs: string;
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
	        this.readingDirection = source["readingDirection"];
	        this.packAs = source["packAs"];
	        this.language = source["language"];
	        this.scanlator = source["scanlator"];
	        this.convertImages = source["convertImages"];
	        this.skipPages = source["skipPages"];
	        this.filenameTemplate = source["filenameTemplate"];
	        this.browserVisible = source["browserVisible"];
	    }
	}
	export class PrefsDTO {
	    uiLanguage?: string;
	    checkUpdates: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PrefsDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.uiLanguage = source["uiLanguage"];
	        this.checkUpdates = source["checkUpdates"];
	    }
	}
	export class RunDTO {
	    chapters: number;
	    bundle: boolean;
	    volumes: boolean;
	    rows: number;
	
	    static createFrom(source: any = {}) {
	        return new RunDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.chapters = source["chapters"];
	        this.bundle = source["bundle"];
	        this.volumes = source["volumes"];
	        this.rows = source["rows"];
	    }
	}
	export class UpdateDTO {
	    latest: string;
	    url: string;
	    oneClick: boolean;
	
	    static createFrom(source: any = {}) {
	        return new UpdateDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.latest = source["latest"];
	        this.url = source["url"];
	        this.oneClick = source["oneClick"];
	    }
	}
	export class VolumesDTO {
	    text: string;
	    site: string;
	    listed: number;
	
	    static createFrom(source: any = {}) {
	        return new VolumesDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.text = source["text"];
	        this.site = source["site"];
	        this.listed = source["listed"];
	    }
	}
	export class SummaryDTO {
	    dir: string;
	    cancelled: boolean;
	    saved: number;
	    volumes: number;
	    parts: boolean;
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
	        this.volumes = source["volumes"];
	        this.parts = source["parts"];
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
	    loadErrorMsg?: Message;
	    title: string;
	    chapterCount: number;
	    hasDuplicates: boolean;
	    hasGroupDuplicates: boolean;
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
	    prefs: PrefsDTO;
	    volumes: VolumesDTO;
	    languages?: FilterDTO;
	    scanlators?: FilterDTO;
	    version: string;
	    releaseURL: string;
	    update?: UpdateDTO;
	    browserVisibleAuto: boolean;
	    browserAlert: string;
	    browserAlertMsg?: Message;
	    filenameTemplateDefault: string;
	    filenameTemplateWarning: string;
	
	    static createFrom(source: any = {}) {
	        return new State(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.loading = source["loading"];
	        this.loaded = source["loaded"];
	        this.loadError = source["loadError"];
	        this.loadErrorMsg = this.convertValues(source["loadErrorMsg"], Message);
	        this.title = source["title"];
	        this.chapterCount = source["chapterCount"];
	        this.hasDuplicates = source["hasDuplicates"];
	        this.hasGroupDuplicates = source["hasGroupDuplicates"];
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
	        this.prefs = this.convertValues(source["prefs"], PrefsDTO);
	        this.volumes = this.convertValues(source["volumes"], VolumesDTO);
	        this.languages = this.convertValues(source["languages"], FilterDTO);
	        this.scanlators = this.convertValues(source["scanlators"], FilterDTO);
	        this.version = source["version"];
	        this.releaseURL = source["releaseURL"];
	        this.update = this.convertValues(source["update"], UpdateDTO);
	        this.browserVisibleAuto = source["browserVisibleAuto"];
	        this.browserAlert = source["browserAlert"];
	        this.browserAlertMsg = this.convertValues(source["browserAlertMsg"], Message);
	        this.filenameTemplateDefault = source["filenameTemplateDefault"];
	        this.filenameTemplateWarning = source["filenameTemplateWarning"];
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

