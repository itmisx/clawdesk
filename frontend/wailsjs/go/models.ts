export namespace agent {
	
	export class Attachment {
	    name: string;
	    type: string;
	    content: string;
	
	    static createFrom(source: any = {}) {
	        return new Attachment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.content = source["content"];
	    }
	}
	export class BotOptions {
	    name: string;
	    avatar: string;
	    description: string;
	    systemPrompt: string;
	    providerId: string;
	    model: string;
	
	    static createFrom(source: any = {}) {
	        return new BotOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.avatar = source["avatar"];
	        this.description = source["description"];
	        this.systemPrompt = source["systemPrompt"];
	        this.providerId = source["providerId"];
	        this.model = source["model"];
	    }
	}
	export class TaskToolCall {
	    toolName: string;
	    args: string;
	    result: string;
	    success: boolean;
	    duration: number;
	
	    static createFrom(source: any = {}) {
	        return new TaskToolCall(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.toolName = source["toolName"];
	        this.args = source["args"];
	        this.result = source["result"];
	        this.success = source["success"];
	        this.duration = source["duration"];
	    }
	}
	export class TaskStep {
	    id: string;
	    name: string;
	    description: string;
	    agentRole: string;
	    status: string;
	    toolCalls: TaskToolCall[];
	    result: string;
	    dependsOn: string[];
	    // Go type: time
	    startAt: any;
	    // Go type: time
	    endAt: any;
	    duration: number;
	
	    static createFrom(source: any = {}) {
	        return new TaskStep(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.agentRole = source["agentRole"];
	        this.status = source["status"];
	        this.toolCalls = this.convertValues(source["toolCalls"], TaskToolCall);
	        this.result = source["result"];
	        this.dependsOn = source["dependsOn"];
	        this.startAt = this.convertValues(source["startAt"], null);
	        this.endAt = this.convertValues(source["endAt"], null);
	        this.duration = source["duration"];
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
	export class TaskPlan {
	    query: string;
	    summary: string;
	    steps: TaskStep[];
	    mermaid: string;
	    // Go type: time
	    startAt: any;
	    // Go type: time
	    endAt: any;
	    duration: number;
	
	    static createFrom(source: any = {}) {
	        return new TaskPlan(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.query = source["query"];
	        this.summary = source["summary"];
	        this.steps = this.convertValues(source["steps"], TaskStep);
	        this.mermaid = source["mermaid"];
	        this.startAt = this.convertValues(source["startAt"], null);
	        this.endAt = this.convertValues(source["endAt"], null);
	        this.duration = source["duration"];
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
	export class ExecutionTrace {
	    sessionId: string;
	    messageTs: string;
	    plan?: TaskPlan;
	
	    static createFrom(source: any = {}) {
	        return new ExecutionTrace(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessionId = source["sessionId"];
	        this.messageTs = source["messageTs"];
	        this.plan = this.convertValues(source["plan"], TaskPlan);
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
	export class FunctionCall {
	    name: string;
	    arguments: string;
	
	    static createFrom(source: any = {}) {
	        return new FunctionCall(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.arguments = source["arguments"];
	    }
	}
	export class ToolCall {
	    id: string;
	    type: string;
	    function: FunctionCall;
	
	    static createFrom(source: any = {}) {
	        return new ToolCall(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.type = source["type"];
	        this.function = this.convertValues(source["function"], FunctionCall);
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
	export class Message {
	    role: string;
	    content: any;
	    reasoning_content?: string;
	    tool_calls?: ToolCall[];
	    tool_call_id?: string;
	    name?: string;
	    timestamp?: string;
	
	    static createFrom(source: any = {}) {
	        return new Message(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.role = source["role"];
	        this.content = source["content"];
	        this.reasoning_content = source["reasoning_content"];
	        this.tool_calls = this.convertValues(source["tool_calls"], ToolCall);
	        this.tool_call_id = source["tool_call_id"];
	        this.name = source["name"];
	        this.timestamp = source["timestamp"];
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
	export class ModelUsage {
	    model: string;
	    promptTokens: number;
	    completionTokens: number;
	    totalTokens: number;
	    requests: number;
	
	    static createFrom(source: any = {}) {
	        return new ModelUsage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.model = source["model"];
	        this.promptTokens = source["promptTokens"];
	        this.completionTokens = source["completionTokens"];
	        this.totalTokens = source["totalTokens"];
	        this.requests = source["requests"];
	    }
	}
	export class NotifyConfig {
	    enabled: boolean;
	    type: string;
	    webhook: string;
	
	    static createFrom(source: any = {}) {
	        return new NotifyConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.type = source["type"];
	        this.webhook = source["webhook"];
	    }
	}
	export class ProviderUsage {
	    providerId: string;
	    providerName: string;
	    promptTokens: number;
	    completionTokens: number;
	    totalTokens: number;
	    requests: number;
	    byModel: Record<string, ModelUsage>;
	
	    static createFrom(source: any = {}) {
	        return new ProviderUsage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.providerId = source["providerId"];
	        this.providerName = source["providerName"];
	        this.promptTokens = source["promptTokens"];
	        this.completionTokens = source["completionTokens"];
	        this.totalTokens = source["totalTokens"];
	        this.requests = source["requests"];
	        this.byModel = this.convertValues(source["byModel"], ModelUsage, true);
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
	export class ScheduleConfig {
	    type: string;
	    interval: number;
	    dailyAt: string;
	    repeatType: string;
	    repeatDays: number;
	    repeatCount: number;
	
	    static createFrom(source: any = {}) {
	        return new ScheduleConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.interval = source["interval"];
	        this.dailyAt = source["dailyAt"];
	        this.repeatType = source["repeatType"];
	        this.repeatDays = source["repeatDays"];
	        this.repeatCount = source["repeatCount"];
	    }
	}
	export class ScheduledTask {
	    id: string;
	    sessionId: string;
	    name: string;
	    prompt: string;
	    enabled: boolean;
	    schedule: ScheduleConfig;
	    notify: NotifyConfig;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    lastRunAt: any;
	    runCount: number;
	    lastResult: string;
	    lastError: string;
	
	    static createFrom(source: any = {}) {
	        return new ScheduledTask(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.sessionId = source["sessionId"];
	        this.name = source["name"];
	        this.prompt = source["prompt"];
	        this.enabled = source["enabled"];
	        this.schedule = this.convertValues(source["schedule"], ScheduleConfig);
	        this.notify = this.convertValues(source["notify"], NotifyConfig);
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.lastRunAt = this.convertValues(source["lastRunAt"], null);
	        this.runCount = source["runCount"];
	        this.lastResult = source["lastResult"];
	        this.lastError = source["lastError"];
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
	export class Session {
	    id: string;
	    name: string;
	    avatar: string;
	    description: string;
	    systemPrompt: string;
	    providerId: string;
	    model: string;
	    // Go type: time
	    createdAt: any;
	    history: Message[];
	
	    static createFrom(source: any = {}) {
	        return new Session(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.avatar = source["avatar"];
	        this.description = source["description"];
	        this.systemPrompt = source["systemPrompt"];
	        this.providerId = source["providerId"];
	        this.model = source["model"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.history = this.convertValues(source["history"], Message);
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
	export class SystemInfo {
	    cpuPercent: number;
	    memUsedMB: number;
	    memTotalMB: number;
	    memPercent: number;
	    storageUsedKB: number;
	    goRoutines: number;
	    embeddingReady: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SystemInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.cpuPercent = source["cpuPercent"];
	        this.memUsedMB = source["memUsedMB"];
	        this.memTotalMB = source["memTotalMB"];
	        this.memPercent = source["memPercent"];
	        this.storageUsedKB = source["storageUsedKB"];
	        this.goRoutines = source["goRoutines"];
	        this.embeddingReady = source["embeddingReady"];
	    }
	}
	
	
	
	
	export class UsageStats {
	    totalPromptTokens: number;
	    totalCompletionTokens: number;
	    totalTokens: number;
	    totalRequests: number;
	    byProvider: Record<string, ProviderUsage>;
	    modelCount: number;
	    skillCount: number;
	    sessionCount: number;
	
	    static createFrom(source: any = {}) {
	        return new UsageStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.totalPromptTokens = source["totalPromptTokens"];
	        this.totalCompletionTokens = source["totalCompletionTokens"];
	        this.totalTokens = source["totalTokens"];
	        this.totalRequests = source["totalRequests"];
	        this.byProvider = this.convertValues(source["byProvider"], ProviderUsage, true);
	        this.modelCount = source["modelCount"];
	        this.skillCount = source["skillCount"];
	        this.sessionCount = source["sessionCount"];
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

export namespace audit {
	
	export class SkillRecord {
	    id: number;
	    timestamp: string;
	    sessionId: string;
	    botName: string;
	    skillName: string;
	    toolName: string;
	    args: string;
	    result: string;
	    success: boolean;
	    durationMs: number;
	
	    static createFrom(source: any = {}) {
	        return new SkillRecord(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.timestamp = source["timestamp"];
	        this.sessionId = source["sessionId"];
	        this.botName = source["botName"];
	        this.skillName = source["skillName"];
	        this.toolName = source["toolName"];
	        this.args = source["args"];
	        this.result = source["result"];
	        this.success = source["success"];
	        this.durationMs = source["durationMs"];
	    }
	}
	export class SkillPageResult {
	    records: SkillRecord[];
	    total: number;
	    page: number;
	    pageSize: number;
	
	    static createFrom(source: any = {}) {
	        return new SkillPageResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.records = this.convertValues(source["records"], SkillRecord);
	        this.total = source["total"];
	        this.page = source["page"];
	        this.pageSize = source["pageSize"];
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
	export class SkillQuery {
	    days: number;
	    toolName: string;
	    page: number;
	    pageSize: number;
	
	    static createFrom(source: any = {}) {
	        return new SkillQuery(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.days = source["days"];
	        this.toolName = source["toolName"];
	        this.page = source["page"];
	        this.pageSize = source["pageSize"];
	    }
	}
	
	export class SkillStats {
	    totalCalls: number;
	    successCalls: number;
	    failedCalls: number;
	    byTool: Record<string, number>;
	    byBot: Record<string, number>;
	
	    static createFrom(source: any = {}) {
	        return new SkillStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.totalCalls = source["totalCalls"];
	        this.successCalls = source["successCalls"];
	        this.failedCalls = source["failedCalls"];
	        this.byTool = source["byTool"];
	        this.byBot = source["byBot"];
	    }
	}
	export class StorageRecord {
	    id: number;
	    timestamp: string;
	    type: string;
	    sessionId: string;
	    fileName: string;
	    detail: string;
	    size: number;
	    count: number;
	    durationMs: number;
	    success: boolean;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new StorageRecord(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.timestamp = source["timestamp"];
	        this.type = source["type"];
	        this.sessionId = source["sessionId"];
	        this.fileName = source["fileName"];
	        this.detail = source["detail"];
	        this.size = source["size"];
	        this.count = source["count"];
	        this.durationMs = source["durationMs"];
	        this.success = source["success"];
	        this.error = source["error"];
	    }
	}
	export class StoragePageResult {
	    records: StorageRecord[];
	    total: number;
	    page: number;
	    pageSize: number;
	
	    static createFrom(source: any = {}) {
	        return new StoragePageResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.records = this.convertValues(source["records"], StorageRecord);
	        this.total = source["total"];
	        this.page = source["page"];
	        this.pageSize = source["pageSize"];
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
	export class StorageQuery {
	    days: number;
	    page: number;
	    pageSize: number;
	
	    static createFrom(source: any = {}) {
	        return new StorageQuery(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.days = source["days"];
	        this.page = source["page"];
	        this.pageSize = source["pageSize"];
	    }
	}
	
	export class StorageStats {
	    totalOps: number;
	    successOps: number;
	    failedOps: number;
	    totalBytes: number;
	    byType: Record<string, number>;
	
	    static createFrom(source: any = {}) {
	        return new StorageStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.totalOps = source["totalOps"];
	        this.successOps = source["successOps"];
	        this.failedOps = source["failedOps"];
	        this.totalBytes = source["totalBytes"];
	        this.byType = source["byType"];
	    }
	}

}

export namespace config {
	
	export class ActiveModel {
	    providerId: string;
	    model: string;
	
	    static createFrom(source: any = {}) {
	        return new ActiveModel(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.providerId = source["providerId"];
	        this.model = source["model"];
	    }
	}
	export class DingtalkConfig {
	    clientId: string;
	    clientSecret: string;
	    lastUserId?: string;
	
	    static createFrom(source: any = {}) {
	        return new DingtalkConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.clientId = source["clientId"];
	        this.clientSecret = source["clientSecret"];
	        this.lastUserId = source["lastUserId"];
	    }
	}
	export class WecomConfig {
	    botId: string;
	    secret: string;
	    lastChatId?: string;
	
	    static createFrom(source: any = {}) {
	        return new WecomConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.botId = source["botId"];
	        this.secret = source["secret"];
	        this.lastChatId = source["lastChatId"];
	    }
	}
	export class FeishuConfig {
	    appId: string;
	    appSecret: string;
	    openId?: string;
	
	    static createFrom(source: any = {}) {
	        return new FeishuConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.appId = source["appId"];
	        this.appSecret = source["appSecret"];
	        this.openId = source["openId"];
	    }
	}
	export class ChannelConfig {
	    id: string;
	    type: string;
	    name: string;
	    enabled: boolean;
	    botId: string;
	    feishu?: FeishuConfig;
	    wecom?: WecomConfig;
	    dingtalk?: DingtalkConfig;
	
	    static createFrom(source: any = {}) {
	        return new ChannelConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.type = source["type"];
	        this.name = source["name"];
	        this.enabled = source["enabled"];
	        this.botId = source["botId"];
	        this.feishu = this.convertValues(source["feishu"], FeishuConfig);
	        this.wecom = this.convertValues(source["wecom"], WecomConfig);
	        this.dingtalk = this.convertValues(source["dingtalk"], DingtalkConfig);
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
	export class ModelProvider {
	    id: string;
	    name: string;
	    baseUrl: string;
	    apiKey: string;
	    models: string[];
	
	    static createFrom(source: any = {}) {
	        return new ModelProvider(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.baseUrl = source["baseUrl"];
	        this.apiKey = source["apiKey"];
	        this.models = source["models"];
	    }
	}
	export class AppConfig {
	    providers: ModelProvider[];
	    activeModel: ActiveModel;
	    channels?: ChannelConfig[];
	
	    static createFrom(source: any = {}) {
	        return new AppConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.providers = this.convertValues(source["providers"], ModelProvider);
	        this.activeModel = this.convertValues(source["activeModel"], ActiveModel);
	        this.channels = this.convertValues(source["channels"], ChannelConfig);
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

export namespace memory {
	
	export class LLMRequestLog {
	    // Go type: time
	    ts: any;
	    systemPrompt: string;
	    tools?: any[];
	
	    static createFrom(source: any = {}) {
	        return new LLMRequestLog(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ts = this.convertValues(source["ts"], null);
	        this.systemPrompt = source["systemPrompt"];
	        this.tools = source["tools"];
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

export namespace skill {
	
	export class ClawHubSkill {
	    name: string;
	    href: string;
	    desc: string;
	
	    static createFrom(source: any = {}) {
	        return new ClawHubSkill(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.href = source["href"];
	        this.desc = source["desc"];
	    }
	}
	export class MCPConfig {
	    transport: string;
	    command: string;
	    args: string[];
	    url: string;
	    env: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new MCPConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.transport = source["transport"];
	        this.command = source["command"];
	        this.args = source["args"];
	        this.url = source["url"];
	        this.env = source["env"];
	    }
	}
	export class PropDef {
	    type: string;
	    description: string;
	    items?: Record<string, any>;
	    enum?: string[];
	
	    static createFrom(source: any = {}) {
	        return new PropDef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.description = source["description"];
	        this.items = source["items"];
	        this.enum = source["enum"];
	    }
	}
	export class ToolExecute {
	    type: string;
	    command: string;
	
	    static createFrom(source: any = {}) {
	        return new ToolExecute(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.command = source["command"];
	    }
	}
	export class ToolParam {
	    type: string;
	    properties: Record<string, PropDef>;
	    required: string[];
	
	    static createFrom(source: any = {}) {
	        return new ToolParam(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.properties = this.convertValues(source["properties"], PropDef, true);
	        this.required = source["required"];
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
	export class Tool {
	    name: string;
	    description: string;
	    parameters: ToolParam;
	    execute: ToolExecute;
	
	    static createFrom(source: any = {}) {
	        return new Tool(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.parameters = this.convertValues(source["parameters"], ToolParam);
	        this.execute = this.convertValues(source["execute"], ToolExecute);
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
	export class Skill {
	    name: string;
	    displayName: string;
	    description: string;
	    version: string;
	    enabled: boolean;
	    builtin: boolean;
	    deferred?: boolean;
	    type?: string;
	    format?: string;
	    mcp?: MCPConfig;
	    content?: string;
	    tools: Tool[];
	    securityLevel?: string;
	    securityNote?: string;
	
	    static createFrom(source: any = {}) {
	        return new Skill(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.displayName = source["displayName"];
	        this.description = source["description"];
	        this.version = source["version"];
	        this.enabled = source["enabled"];
	        this.builtin = source["builtin"];
	        this.deferred = source["deferred"];
	        this.type = source["type"];
	        this.format = source["format"];
	        this.mcp = this.convertValues(source["mcp"], MCPConfig);
	        this.content = source["content"];
	        this.tools = this.convertValues(source["tools"], Tool);
	        this.securityLevel = source["securityLevel"];
	        this.securityNote = source["securityNote"];
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
	export class SkillHubSkill {
	    name: string;
	    slug: string;
	    category: string;
	    description: string;
	    description_zh: string;
	    version: string;
	    ownerName: string;
	    score: number;
	    stars: number;
	    downloads: number;
	    tags: string[];
	
	    static createFrom(source: any = {}) {
	        return new SkillHubSkill(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.slug = source["slug"];
	        this.category = source["category"];
	        this.description = source["description"];
	        this.description_zh = source["description_zh"];
	        this.version = source["version"];
	        this.ownerName = source["ownerName"];
	        this.score = source["score"];
	        this.stars = source["stars"];
	        this.downloads = source["downloads"];
	        this.tags = source["tags"];
	    }
	}
	
	

}

