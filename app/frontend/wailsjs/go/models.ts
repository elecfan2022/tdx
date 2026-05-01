export namespace main {
	
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

