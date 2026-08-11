import type { Event as GeneratedEvent, EventMembership, User } from '$lib/api/generated';

export type Organizer = User;

export type Event = GeneratedEvent;

export interface EventSeries {
	id: string;
	organizerId: string;
	title: string;
	description: string;
	location: string;
	timezone: string;
	eventTime: string;
	durationMinutes?: number;
	recurrenceRule: 'weekly' | 'biweekly' | 'monthly';
	recurrenceEnd?: string;
	maxOccurrences?: number;
	seriesStatus: 'active' | 'stopped';
	retentionDays: number;
	showHeadcount: boolean;
	showGuestList: boolean;
	rsvpDeadlineOffsetHours?: number;
	createdAt: string;
	updatedAt: string;
}

export interface InviteCard {
	id: string;
	eventId: string;
	templateId: string;
	heading: string;
	body: string;
	footer: string;
	primaryColor: string;
	secondaryColor: string;
	font: string;
	customData: Record<string, unknown>;
	createdAt: string;
	updatedAt: string;
}

export interface Reminder {
	id: string;
	eventId: string;
	remindAt: string;
	targetGroup: 'all' | 'attending' | 'maybe' | 'declined' | 'pending';
	message: string;
	status: 'scheduled' | 'sent' | 'cancelled' | 'failed';
	createdAt: string;
	updatedAt: string;
}

export type CoHost = Omit<EventMembership, 'role'> & {
	role: 'cohost';
	organizerEmail?: string;
	organizerName?: string;
};

export interface ApiError {
	error: string;
	message: string;
	status: number;
}

export interface ApiResponse<T> {
	data: T;
}

export interface PaginatedResponse<T> {
	data: T[];
	total: number;
	page: number;
	perPage: number;
}

export interface EventQuestion {
	id: string;
	eventId: string;
	label: string;
	type: 'text' | 'select' | 'checkbox';
	options: string[];
	required: boolean;
	scope: 'invitation' | 'guest';
	sortOrder: number;
	createdAt: string;
	updatedAt: string;
}

export type InvitationAttendance = 'pending' | 'attending' | 'maybe' | 'declined';

export interface InvitationGuest {
	id: string;
	invitationId: string;
	name: string;
	origin: 'assigned' | 'additional';
	sortOrder: number;
	attendance: InvitationAttendance;
}

export interface InvitationRecord {
	id: string;
	eventId: string;
	label: string;
	contactEmail?: string;
	contactPhone?: string;
	preferredDeliveryMethod: 'email' | 'sms' | 'none';
	additionalGuestAllowance: number;
	source: 'private' | 'open';
	tokenVersion: number;
	revokedAt?: string;
	createdAt: string;
	updatedAt: string;
}

export interface InvitationQuestion {
	id: string;
	label: string;
	type: 'text' | 'select' | 'checkbox';
	options: string[];
	required: boolean;
	scope: 'invitation' | 'guest';
	sortOrder: number;
}

export interface InvitationHousehold {
	invitation: InvitationRecord;
	event: {
		id: string;
		title: string;
		description: string;
		eventDate: string;
		endDate?: string;
		location: string;
		timezone: string;
		status: string;
	};
	response: { id: string; invitationId: string; version: number; submittedAt?: string };
	guests: InvitationGuest[];
	questions: InvitationQuestion[];
	invitationAnswers: Array<{ questionId: string; answer: string }>;
	guestAnswers: Array<{ guestId: string; questionId: string; answer: string }>;
}

export interface InvitationImportIssue {
	row?: number;
	field?: string;
	message: string;
}

export interface InvitationImportHousehold {
	householdKey: string;
	householdLabel: string;
	contactEmail?: string;
	contactPhone?: string;
	preferredDelivery: 'email' | 'sms' | 'none';
	additionalGuestAllowance: number;
	assignedGuestNames: string[];
}

export interface InvitationImportPreview {
	householdCount: number;
	assignedGuestCount: number;
	households: InvitationImportHousehold[];
	errors: InvitationImportIssue[];
	warnings: InvitationImportIssue[];
}

export type InviteTemplate = {
	id: string;
	name: string;
	description: string;
	previewImage: string;
};

export interface Webhook {
	id: string;
	eventId: string;
	url: string;
	secret?: string;
	eventTypes: string[];
	description: string;
	enabled: boolean;
	createdAt: string;
	updatedAt: string;
}

export interface WebhookWithSecret extends Webhook {
	secret: string;
}

export interface WebhookDelivery {
	id: string;
	webhookId: string;
	eventType: string;
	payload: string;
	responseStatus?: number;
	responseBody?: string;
	error?: string;
	attempt: number;
	deliveredAt?: string;
	createdAt: string;
}

export interface EmailStats {
	totalSent: number;
	delivered: number;
	opened: number;
	clicked: number;
	bounced: number;
	complained: number;
	failed: number;
	pending: number;
}

export interface InstanceStats {
	events: {
		total: number;
		draft: number;
		published: number;
		cancelled: number;
		archived: number;
	};
	guests: {
		total: number;
		totalHeadcount: number;
		attending: number;
		maybe: number;
		declined: number;
		pending: number;
		avgPerEvent: number;
	};
	users: {
		total: number;
	};
	features: {
		openEnrollmentEvents: number;
		cohostedEvents: number;
		eventsWithQuestions: number;
		eventsWithCapacity: number;
		seriesEvents: number;
	};
	notifications: {
		total: number;
		sent: number;
		failed: number;
		delivered: number;
		opened: number;
		bounced: number;
		complained: number;
	};
}

export interface NotificationLogEntry {
	id: string;
	eventId: string;
	invitationId: string;
	channel: string;
	provider: string;
	status: string;
	deliveryStatus: string;
	error: string;
	recipient: string;
	subject: string;
	messageId: string;
	sentAt?: string;
	deliveredAt?: string;
	openedAt?: string;
	clickedAt?: string;
	bouncedAt?: string;
	bounceType: string;
	complaintAt?: string;
	createdAt: string;
}
