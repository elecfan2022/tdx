export namespace main {
	
	export class Fractal {
	    type: string;
	    index: number;
	    timestamp: number;
	    price: number;
	    origStartIdx: number;
	    origEndIdx: number;
	    peakIdx: number;
	    kHigh: number;
	    kLow: number;
	    isEndpoint: boolean;
	    leftIdx: number;
	    rightIdx: number;
	
	    static createFrom(source: any = {}) {
	        return new Fractal(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.index = source["index"];
	        this.timestamp = source["timestamp"];
	        this.price = source["price"];
	        this.origStartIdx = source["origStartIdx"];
	        this.origEndIdx = source["origEndIdx"];
	        this.peakIdx = source["peakIdx"];
	        this.kHigh = source["kHigh"];
	        this.kLow = source["kLow"];
	        this.isEndpoint = source["isEndpoint"];
	        this.leftIdx = source["leftIdx"];
	        this.rightIdx = source["rightIdx"];
	    }
	}
	export class Bi {
	    from: Fractal;
	    to: Fractal;
	
	    static createFrom(source: any = {}) {
	        return new Bi(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.from = this.convertValues(source["from"], Fractal);
	        this.to = this.convertValues(source["to"], Fractal);
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
	export class BiDiagnosis {
	    fromFound: boolean;
	    toFound: boolean;
	    from?: Fractal;
	    to?: Fractal;
	    indexDist: number;
	    peakDist: number;
	    rule1: string;
	    rule2: string;
	    rule3: string;
	    allPass: boolean;
	    note: string;
	
	    static createFrom(source: any = {}) {
	        return new BiDiagnosis(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.fromFound = source["fromFound"];
	        this.toFound = source["toFound"];
	        this.from = this.convertValues(source["from"], Fractal);
	        this.to = this.convertValues(source["to"], Fractal);
	        this.indexDist = source["indexDist"];
	        this.peakDist = source["peakDist"];
	        this.rule1 = source["rule1"];
	        this.rule2 = source["rule2"];
	        this.rule3 = source["rule3"];
	        this.allPass = source["allPass"];
	        this.note = source["note"];
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
	
	export class KlineBar {
	    timestamp: number;
	    open: number;
	    high: number;
	    low: number;
	    close: number;
	    volume: number;
	    turnover: number;
	
	    static createFrom(source: any = {}) {
	        return new KlineBar(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.timestamp = source["timestamp"];
	        this.open = source["open"];
	        this.high = source["high"];
	        this.low = source["low"];
	        this.close = source["close"];
	        this.volume = source["volume"];
	        this.turnover = source["turnover"];
	    }
	}
	export class KlineWithChan {
	    klines: KlineBar[];
	    fractals: Fractal[];
	    bis: Bi[];
	
	    static createFrom(source: any = {}) {
	        return new KlineWithChan(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.klines = this.convertValues(source["klines"], KlineBar);
	        this.fractals = this.convertValues(source["fractals"], Fractal);
	        this.bis = this.convertValues(source["bis"], Bi);
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
	export class Status {
	    connected: boolean;
	    codesReady: boolean;
	    codesError: string;
	    dialError: string;
	    stockCount: number;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.connected = source["connected"];
	        this.codesReady = source["codesReady"];
	        this.codesError = source["codesError"];
	        this.dialError = source["dialError"];
	        this.stockCount = source["stockCount"];
	    }
	}
	export class StockInfo {
	    code: string;
	    fullCode: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new StockInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.code = source["code"];
	        this.fullCode = source["fullCode"];
	        this.name = source["name"];
	    }
	}
	export class WatchItem {
	    code: string;
	    fullCode: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new WatchItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.code = source["code"];
	        this.fullCode = source["fullCode"];
	        this.name = source["name"];
	    }
	}

}

