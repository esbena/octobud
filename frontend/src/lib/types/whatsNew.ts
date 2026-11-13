export type WhatsNewAuthorType = "human" | "bot" | "unknown";

/** Raw evidence from the complete, non-summarizing reader collector. */
export interface WhatsNewEvidence {
	id: string;
	kind: string;
	change: string;
	at?: string;
	createdAt?: string;
	updatedAt?: string;
	actor?: string;
	actorType?: WhatsNewAuthorType;
	body?: string;
	state?: string;
	url?: string;
	parentId?: string;
	rootId?: string;
	reviewId?: string;
	detail?: string;
	path?: string;
	diffHunk?: string;
	outdated?: boolean;
	message?: string;
}

export interface WhatsNewSnapshot {
	notificationId: string;
	title: string;
	selection?: string;
	since: string;
	baselineMessage?: string;
	fetchedAt: string;
	totalEntries: number;
	newActivity: WhatsNewEvidence[];
	olderContext: WhatsNewEvidence[];
	warnings: string[];
}

export interface WhatsNewEntry {
	id: string;
	eventKind: string;
	author: { login: string; type: WhatsNewAuthorType };
	body: string;
	createdAt?: string;
	updatedAt?: string;
	activityAt?: string;
	htmlUrl?: string;
	change: string;
	isNew: boolean;
	state?: string;
	detail?: string;
	path?: string;
	diffHunk?: string;
	outdated?: boolean;
	message?: string;
}

export type WhatsNewThreadItem =
	| { kind: "entry"; entry: WhatsNewEntry }
	| { kind: "omitted"; count: number };

export interface WhatsNewThread {
	id: string;
	path?: string;
	htmlUrl?: string;
	authorType: WhatsNewAuthorType;
	entries: WhatsNewThreadItem[];
}

export type WhatsNewFeedItem =
	| { kind: "entry"; entry: WhatsNewEntry }
	| { kind: "thread"; thread: WhatsNewThread };

export interface WhatsNewEntryCounts {
	full: number;
	whatsNew: number;
}

/** Active comment streams begin with at most one old context entry, followed by new entries. */
export interface WhatsNewReaderModel {
	counts: WhatsNewEntryCounts;
	cutoff: string;
	baselineMessage?: string;
	latestActivityAt?: string;
	warnings: string[];
	humanFeed: WhatsNewFeedItem[];
	botFeed: WhatsNewFeedItem[];
	humanComments: WhatsNewEntry[];
	humanThreads: WhatsNewThread[];
	humanActivity: WhatsNewEntry[];
	nonHumanComments: WhatsNewEntry[];
	nonHumanThreads: WhatsNewThread[];
	nonHumanActivity: WhatsNewEntry[];
	unknownComments: WhatsNewEntry[];
	unknownThreads: WhatsNewThread[];
	unknownActivity: WhatsNewEntry[];
}
