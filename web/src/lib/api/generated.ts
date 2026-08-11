// Generated from api/openapi.json by scripts/generate-api-client.mjs.
// Do not edit by hand; run `npm run generate:api` from web/.

export type Error = {
	"error": string;
	"message"?: string;
};

export type User = {
	"id": string;
	"email": string;
	"name": string;
	"timezone": string;
	"instanceRole": "admin" | "user";
	"status": "invited" | "active" | "disabled";
	"isAdmin": boolean;
	"invitedByUserId"?: string;
	"activatedAt"?: string;
	"lastLoginAt"?: string;
	"createdAt": string;
	"updatedAt": string;
};

export type InstanceSettings = {
	"instanceName": string;
	"defaultTimezone": string;
	"allowSignups": boolean;
	"supportEmail": string;
	"configured": boolean;
};

export type InstanceSettingsUpdate = {
	"instanceName": string;
	"defaultTimezone": string;
	"allowSignups": boolean;
	"supportEmail": string;
};

export type SetupStatus = {
	"configured": boolean;
	"setupRequired": boolean;
};

export type BootstrapRequest = {
	"bootstrapToken": string;
	"adminEmail": string;
	"adminName": string;
	"instanceName": string;
	"defaultTimezone": string;
	"allowSignups": boolean;
	"supportEmail": string;
};

export type AccountInvite = {
	"id": string;
	"targetUserId": string;
	"email": string;
	"invitedByUserId": string;
	"eventId"?: string;
	"eventRole"?: "cohost";
	"expiresAt": string;
	"acceptedAt"?: string;
	"revokedAt"?: string;
	"createdAt": string;
};

export type AuditEntry = {
	"id": string;
	"actorUserId"?: string;
	"actorKind": "user" | "cli" | "system";
	"action": string;
	"targetUserId"?: string;
	"eventId"?: string;
	"metadata": Record<string, unknown>;
	"createdAt": string;
};

export type Event = {
	"id": string;
	"organizerId": string;
	"title": string;
	"description": string;
	"eventDate": string;
	"endDate"?: string;
	"location": string;
	"timezone": string;
	"retentionDays": number;
	"status": "draft" | "published" | "cancelled" | "archived";
	"showHeadcount": boolean;
	"showGuestList": boolean;
	"rsvpDeadline"?: string;
	"seriesId"?: string;
	"seriesIndex"?: number;
	"seriesOverride": boolean;
	"createdAt": string;
	"updatedAt": string;
};

export type EventMembership = {
	"id": string;
	"eventId": string;
	"userId": string;
	"role": "owner" | "cohost";
	"grantedByUserId": string;
	"email"?: string;
	"name"?: string;
	"createdAt": string;
};

export type Invitation = {
	"id": string;
	"eventId": string;
	"label": string;
	"contactEmail"?: string;
	"contactPhone"?: string;
	"preferredDeliveryMethod": "email" | "sms" | "none";
	"additionalGuestAllowance": number;
	"source": "private" | "open";
	"tokenVersion": number;
	"revokedAt"?: string;
	"createdAt": string;
	"updatedAt": string;
};

export type InvitationGuest = {
	"id": string;
	"invitationId": string;
	"name": string;
	"origin": "assigned" | "additional";
	"attendance": "pending" | "attending" | "maybe" | "declined";
};

export type InvitationQuestion = {
	"id": string;
	"label": string;
	"type": "text" | "select" | "checkbox";
	"options": Array<string>;
	"required": boolean;
	"scope": "invitation" | "guest";
	"sortOrder": number;
};

export type GuestInvitationPresentation = {
	"templateId": string;
	"heading": string;
	"body": string;
	"footer": string;
	"primaryColor": string;
	"secondaryColor": string;
	"font": string;
	"backgroundImage"?: string;
};

export type InvitationDeliverySummary = {
	"status": "pending" | "sent" | "failed";
	"deliveryStatus": "unknown" | "delivered" | "opened" | "clicked" | "bounced" | "complained";
	"provider": string;
	"error"?: string;
	"attemptedAt": string;
	"sentAt"?: string;
};

export type InvitationHousehold = {
	"invitation": Invitation;
	"event": Record<string, unknown>;
	"response": Record<string, unknown>;
	"guests": Array<InvitationGuest>;
	"questions": Array<InvitationQuestion>;
	"invitationAnswers": Array<Record<string, unknown>>;
	"guestAnswers": Array<Record<string, unknown>>;
	"presentation": GuestInvitationPresentation;
	"latestDelivery"?: InvitationDeliverySummary;
};

export type CreateInvitationRequest = {
	"label": string;
	"contactEmail"?: string;
	"contactPhone"?: string;
	"preferredDeliveryMethod": "email" | "sms" | "none";
	"additionalGuestAllowance": number;
	"assignedGuestNames": Array<string>;
	"send"?: boolean;
};

export type UpdateInvitationRequest = {
	"label": string;
	"contactEmail"?: string;
	"contactPhone"?: string;
	"preferredDeliveryMethod": "email" | "sms" | "none";
	"additionalGuestAllowance": number;
	"assignedGuests": Array<{
		"id"?: string;
		"name": string;
	}>;
};

export type InvitationDeliveryResult = {
	"status": "not_requested" | "sent" | "failed";
	"warning"?: string;
};

export type CreateInvitationResult = {
	"invitation": Invitation;
	"guests": Array<InvitationGuest>;
	"accessUrl": string;
	"delivery": InvitationDeliveryResult;
};

export type InvitationCapabilityRequest = {
	"capability": string;
};

export type InvitationSubmitRequest = {
	"version": number;
	"assignedGuests": Array<AssignedGuestAttendanceInput>;
	"additionalGuests": Array<AdditionalGuestResponseInput>;
	"invitationAnswers": Record<string, unknown>;
	"guestAnswers": Record<string, unknown>;
};

export type AssignedGuestAttendanceInput = {
	"guestId": string;
	"attendance": "pending" | "attending" | "maybe" | "declined";
};

export type AdditionalGuestResponseInput = {
	"id"?: string;
	"clientKey"?: string;
	"name": string;
	"attendance": "pending" | "attending" | "maybe" | "declined";
};

export type RecoveryRequest = {
	"eventId": string;
	"contact": string;
};

export type OpenEnrollmentConfigRequest = {
	"enabled": boolean;
	"opensAt"?: string;
	"closesAt"?: string;
	"maxPartySize": number;
	"capacity"?: number;
};

export type OpenEnrollmentConfig = {
	"id": string;
	"eventId": string;
	"enabled": boolean;
	"opensAt"?: string;
	"closesAt"?: string;
	"maxPartySize": number;
	"capacity"?: number;
	"tokenVersion": number;
	"revokedAt"?: string;
	"createdAt": string;
	"updatedAt": string;
};

export type OpenEnrollmentResult = {
	"config": OpenEnrollmentConfig;
	"accessUrl": string;
};

export type InvitationMessageRequest = {
	"recipientGroup": "all" | "attending" | "maybe" | "declined" | "pending";
	"subject": string;
	"body": string;
};

export type InvitationMessageResult = {
	"attempted": number;
	"accepted": number;
	"failed": number;
	"skipped": number;
};

export type InvitationMessagePreviewRequest = {
	"recipientGroup": "all" | "attending" | "maybe" | "declined" | "pending";
};

export type InvitationMessagePreview = {
	"recipientGroup": "all" | "attending" | "maybe" | "declined" | "pending";
	"recipientHouseholds": number;
};

export type InvitationImportIssue = {
	"row"?: number;
	"field"?: string;
	"message": string;
};

export type InvitationImportHousehold = {
	"householdKey": string;
	"householdLabel": string;
	"contactEmail"?: string;
	"contactPhone"?: string;
	"preferredDelivery": "email" | "sms" | "none";
	"additionalGuestAllowance": number;
	"assignedGuestNames": Array<string>;
};

export type InvitationImportPreview = {
	"householdCount": number;
	"assignedGuestCount": number;
	"households": Array<InvitationImportHousehold>;
	"errors": Array<InvitationImportIssue>;
	"warnings": Array<InvitationImportIssue>;
};

export type InvitationImportCommitRequest = {
	"households": Array<InvitationImportHousehold>;
};

export type InvitationImportCommitResult = {
	"householdCount": number;
	"assignedGuestCount": number;
	"invitationIds": Array<string>;
};

export type Reminder = {
	"id": string;
	"eventId": string;
	"remindAt": string;
	"targetGroup": "all" | "attending" | "maybe" | "declined" | "pending";
	"message": string;
	"status": "scheduled" | "processing" | "sent" | "cancelled" | "failed";
	"createdAt": string;
	"updatedAt": string;
};

export type CreateReminderRequest = {
	"remindAt": string;
	"targetGroup": "all" | "attending" | "maybe" | "declined" | "pending";
	"message": string;
};

export type UpdateReminderRequest = {
	"remindAt"?: string;
	"targetGroup"?: "all" | "attending" | "maybe" | "declined" | "pending";
	"message"?: string;
};

export type OpenEnrollmentRequest = {
	"capability": string;
	"label": string;
	"contactEmail": string;
	"contactPhone"?: string;
	"preferredDeliveryMethod": "email";
	"guestNames": Array<string>;
};

export type OpenEnrollmentCreateResult = {
	"data": InvitationHousehold;
	"delivery": InvitationDeliveryResult;
};

export interface Operations {
	getOpenAPIContract: {
		parameters: void;
		requestBody: void;
		response: Record<string, unknown>;
	};
	getSetupStatus: {
		parameters: void;
		requestBody: void;
		response: SetupStatus;
	};
	bootstrapInstance: {
		parameters: void;
		requestBody: BootstrapRequest;
		response: {
	"data": {
		"user": User;
		"configured": boolean;
	};
};
	};
	getInstanceSettings: {
		parameters: void;
		requestBody: void;
		response: {
	"data": InstanceSettings;
};
	};
	updateInstanceSettings: {
		parameters: void;
		requestBody: InstanceSettingsUpdate;
		response: {
	"data": InstanceSettings;
};
	};
	requestMagicLink: {
		parameters: void;
		requestBody: {
	"email": string;
};
		response: {
	"message": string;
};
	};
	verifyMagicLink: {
		parameters: void;
		requestBody: {
	"token": string;
};
		response: {
	"token": string;
	"organizer": User;
};
	};
	getCurrentUser: {
		parameters: void;
		requestBody: void;
		response: User;
	};
	updateCurrentUser: {
		parameters: void;
		requestBody: {
	"name"?: string;
	"timezone"?: string;
};
		response: User;
	};
	acceptAccountInvite: {
		parameters: void;
		requestBody: {
	"token": string;
};
		response: {
	"data": {
		"user": User;
	};
};
	};
	listUsers: {
		parameters: void;
		requestBody: void;
		response: {
	"data": {
		"users": Array<User>;
	};
};
	};
	listAccountInvites: {
		parameters: void;
		requestBody: void;
		response: {
	"data": {
		"invites": Array<AccountInvite>;
	};
};
	};
	inviteUser: {
		parameters: void;
		requestBody: {
	"email": string;
};
		response: {
	"data": AccountInvite;
};
	};
	revokeAccountInvite: {
		parameters: {
	"inviteId": string;
};
		requestBody: void;
		response: void;
	};
	updateUserStatus: {
		parameters: {
	"userId": string;
};
		requestBody: {
	"status": "active" | "disabled";
};
		response: void;
	};
	updateUserRole: {
		parameters: {
	"userId": string;
};
		requestBody: {
	"instanceRole": "admin" | "user";
};
		response: void;
	};
	listAdminAudit: {
		parameters: {
	"limit"?: number;
};
		requestBody: void;
		response: {
	"data": {
		"entries": Array<AuditEntry>;
	};
};
	};
	addAdminEventMembership: {
		parameters: {
	"eventId": string;
};
		requestBody: void;
		response: void;
	};
	transferEventOwnership: {
		parameters: {
	"eventId": string;
};
		requestBody: {
	"newOwnerUserId": string;
};
		response: void;
	};
	listEvents: {
		parameters: void;
		requestBody: void;
		response: {
	"data": Array<Event>;
};
	};
	listEventCohosts: {
		parameters: {
	"eventId": string;
};
		requestBody: void;
		response: {
	"data": Array<EventMembership>;
};
	};
	inviteEventCohost: {
		parameters: {
	"eventId": string;
};
		requestBody: {
	"email": string;
};
		response: void;
	};
	listInvitations: {
		parameters: {
	"eventId": string;
	"search"?: string;
	"response"?: "submitted" | "not_submitted";
	"attendance"?: "pending" | "attending" | "maybe" | "declined";
};
		requestBody: void;
		response: {
	"data": Array<InvitationHousehold>;
};
	};
	createInvitation: {
		parameters: {
	"eventId": string;
};
		requestBody: CreateInvitationRequest;
		response: {
	"data": CreateInvitationResult;
};
	};
	downloadInvitationImportTemplate: {
		parameters: {
	"eventId": string;
};
		requestBody: void;
		response: void;
	};
	previewInvitationImport: {
		parameters: {
	"eventId": string;
};
		requestBody: void;
		response: {
	"data": InvitationImportPreview;
};
	};
	commitInvitationImport: {
		parameters: {
	"eventId": string;
};
		requestBody: InvitationImportCommitRequest;
		response: {
	"data": InvitationImportCommitResult;
};
	};
	exportInvitationResponses: {
		parameters: {
	"eventId": string;
};
		requestBody: void;
		response: void;
	};
	getInvitation: {
		parameters: {
	"eventId": string;
	"invitationId": string;
};
		requestBody: void;
		response: {
	"data": InvitationHousehold;
};
	};
	updateInvitationHousehold: {
		parameters: {
	"eventId": string;
	"invitationId": string;
};
		requestBody: UpdateInvitationRequest;
		response: {
	"data": InvitationHousehold;
};
	};
	deliverInvitation: {
		parameters: {
	"eventId": string;
	"invitationId": string;
};
		requestBody: void;
		response: void;
	};
	rotateInvitationCapability: {
		parameters: {
	"eventId": string;
	"invitationId": string;
};
		requestBody: void;
		response: {
	"data": CreateInvitationResult;
};
	};
	revokeInvitation: {
		parameters: {
	"eventId": string;
	"invitationId": string;
};
		requestBody: {
	"reason"?: string;
};
		response: void;
	};
	messageInvitationHouseholds: {
		parameters: {
	"eventId": string;
};
		requestBody: InvitationMessageRequest;
		response: {
	"data": InvitationMessageResult;
};
	};
	previewInvitationHouseholdMessage: {
		parameters: {
	"eventId": string;
};
		requestBody: InvitationMessagePreviewRequest;
		response: {
	"data": InvitationMessagePreview;
};
	};
	listEventReminders: {
		parameters: {
	"eventId": string;
};
		requestBody: void;
		response: {
	"data": Array<Reminder>;
};
	};
	createEventReminder: {
		parameters: {
	"eventId": string;
};
		requestBody: CreateReminderRequest;
		response: {
	"data": Reminder;
};
	};
	updateEventReminder: {
		parameters: {
	"reminderId": string;
};
		requestBody: UpdateReminderRequest;
		response: {
	"data": Reminder;
};
	};
	cancelEventReminder: {
		parameters: {
	"reminderId": string;
};
		requestBody: void;
		response: Record<string, unknown>;
	};
	getOpenEnrollment: {
		parameters: {
	"eventId": string;
};
		requestBody: void;
		response: {
	"data": OpenEnrollmentResult | null;
};
	};
	configureOpenEnrollment: {
		parameters: {
	"eventId": string;
};
		requestBody: OpenEnrollmentConfigRequest;
		response: {
	"data": OpenEnrollmentResult;
};
	};
	rotateOpenEnrollmentCapability: {
		parameters: {
	"eventId": string;
};
		requestBody: void;
		response: {
	"data": OpenEnrollmentResult;
};
	};
	exchangeInvitationCapability: {
		parameters: void;
		requestBody: InvitationCapabilityRequest;
		response: {
	"data": InvitationHousehold;
};
	};
	getInvitationSession: {
		parameters: void;
		requestBody: void;
		response: {
	"data": InvitationHousehold;
};
	};
	submitInvitationResponse: {
		parameters: void;
		requestBody: InvitationSubmitRequest;
		response: void;
	};
	requestInvitationRecovery: {
		parameters: void;
		requestBody: RecoveryRequest;
		response: void;
	};
	exchangeInvitationRecovery: {
		parameters: void;
		requestBody: InvitationCapabilityRequest;
		response: void;
	};
	inspectOpenEnrollment: {
		parameters: void;
		requestBody: InvitationCapabilityRequest;
		response: void;
	};
	enrollOpenInvitation: {
		parameters: void;
		requestBody: OpenEnrollmentRequest;
		response: OpenEnrollmentCreateResult;
	};
}

export const operationDefinitions = {
	getOpenAPIContract: {"method":"GET","path":"/openapi.json","pathParams":[],"queryParams":[]},
	getSetupStatus: {"method":"GET","path":"/setup/status","pathParams":[],"queryParams":[]},
	bootstrapInstance: {"method":"POST","path":"/setup/bootstrap","pathParams":[],"queryParams":[]},
	getInstanceSettings: {"method":"GET","path":"/setup/config","pathParams":[],"queryParams":[]},
	updateInstanceSettings: {"method":"POST","path":"/setup/config","pathParams":[],"queryParams":[]},
	requestMagicLink: {"method":"POST","path":"/auth/magic-link","pathParams":[],"queryParams":[]},
	verifyMagicLink: {"method":"POST","path":"/auth/verify","pathParams":[],"queryParams":[]},
	getCurrentUser: {"method":"GET","path":"/auth/me","pathParams":[],"queryParams":[]},
	updateCurrentUser: {"method":"PATCH","path":"/auth/me","pathParams":[],"queryParams":[]},
	acceptAccountInvite: {"method":"POST","path":"/auth/account-invites/accept","pathParams":[],"queryParams":[]},
	listUsers: {"method":"GET","path":"/admin/users","pathParams":[],"queryParams":[]},
	listAccountInvites: {"method":"GET","path":"/admin/users/invites","pathParams":[],"queryParams":[]},
	inviteUser: {"method":"POST","path":"/admin/users/invites","pathParams":[],"queryParams":[]},
	revokeAccountInvite: {"method":"DELETE","path":"/admin/users/invites/{inviteId}","pathParams":["inviteId"],"queryParams":[]},
	updateUserStatus: {"method":"PATCH","path":"/admin/users/{userId}/status","pathParams":["userId"],"queryParams":[]},
	updateUserRole: {"method":"PATCH","path":"/admin/users/{userId}/role","pathParams":["userId"],"queryParams":[]},
	listAdminAudit: {"method":"GET","path":"/admin/audit","pathParams":[],"queryParams":["limit"]},
	addAdminEventMembership: {"method":"POST","path":"/admin/events/{eventId}/memberships/self","pathParams":["eventId"],"queryParams":[]},
	transferEventOwnership: {"method":"POST","path":"/admin/events/{eventId}/ownership-transfer","pathParams":["eventId"],"queryParams":[]},
	listEvents: {"method":"GET","path":"/events","pathParams":[],"queryParams":[]},
	listEventCohosts: {"method":"GET","path":"/events/{eventId}/cohosts","pathParams":["eventId"],"queryParams":[]},
	inviteEventCohost: {"method":"POST","path":"/events/{eventId}/cohosts","pathParams":["eventId"],"queryParams":[]},
	listInvitations: {"method":"GET","path":"/events/{eventId}/invitations","pathParams":["eventId"],"queryParams":["search","response","attendance"]},
	createInvitation: {"method":"POST","path":"/events/{eventId}/invitations","pathParams":["eventId"],"queryParams":[]},
	downloadInvitationImportTemplate: {"method":"GET","path":"/events/{eventId}/invitations/import/template","pathParams":["eventId"],"queryParams":[]},
	previewInvitationImport: {"method":"POST","path":"/events/{eventId}/invitations/import/preview","pathParams":["eventId"],"queryParams":[]},
	commitInvitationImport: {"method":"POST","path":"/events/{eventId}/invitations/import/commit","pathParams":["eventId"],"queryParams":[]},
	exportInvitationResponses: {"method":"GET","path":"/events/{eventId}/invitations/export","pathParams":["eventId"],"queryParams":[]},
	getInvitation: {"method":"GET","path":"/events/{eventId}/invitations/{invitationId}","pathParams":["eventId","invitationId"],"queryParams":[]},
	updateInvitationHousehold: {"method":"PUT","path":"/events/{eventId}/invitations/{invitationId}","pathParams":["eventId","invitationId"],"queryParams":[]},
	deliverInvitation: {"method":"POST","path":"/events/{eventId}/invitations/{invitationId}/deliver","pathParams":["eventId","invitationId"],"queryParams":[]},
	rotateInvitationCapability: {"method":"POST","path":"/events/{eventId}/invitations/{invitationId}/rotate","pathParams":["eventId","invitationId"],"queryParams":[]},
	revokeInvitation: {"method":"POST","path":"/events/{eventId}/invitations/{invitationId}/revoke","pathParams":["eventId","invitationId"],"queryParams":[]},
	messageInvitationHouseholds: {"method":"POST","path":"/events/{eventId}/invitations/messages","pathParams":["eventId"],"queryParams":[]},
	previewInvitationHouseholdMessage: {"method":"POST","path":"/events/{eventId}/invitations/messages/preview","pathParams":["eventId"],"queryParams":[]},
	listEventReminders: {"method":"GET","path":"/reminders/event/{eventId}","pathParams":["eventId"],"queryParams":[]},
	createEventReminder: {"method":"POST","path":"/reminders/event/{eventId}","pathParams":["eventId"],"queryParams":[]},
	updateEventReminder: {"method":"PUT","path":"/reminders/{reminderId}","pathParams":["reminderId"],"queryParams":[]},
	cancelEventReminder: {"method":"DELETE","path":"/reminders/{reminderId}","pathParams":["reminderId"],"queryParams":[]},
	getOpenEnrollment: {"method":"GET","path":"/events/{eventId}/open-enrollment","pathParams":["eventId"],"queryParams":[]},
	configureOpenEnrollment: {"method":"PUT","path":"/events/{eventId}/open-enrollment","pathParams":["eventId"],"queryParams":[]},
	rotateOpenEnrollmentCapability: {"method":"POST","path":"/events/{eventId}/open-enrollment/rotate","pathParams":["eventId"],"queryParams":[]},
	exchangeInvitationCapability: {"method":"POST","path":"/invitations/exchange","pathParams":[],"queryParams":[]},
	getInvitationSession: {"method":"GET","path":"/invitations/session","pathParams":[],"queryParams":[]},
	submitInvitationResponse: {"method":"PUT","path":"/invitations/session/response","pathParams":[],"queryParams":[]},
	requestInvitationRecovery: {"method":"POST","path":"/invitations/recovery/request","pathParams":[],"queryParams":[]},
	exchangeInvitationRecovery: {"method":"POST","path":"/invitations/recovery/exchange","pathParams":[],"queryParams":[]},
	inspectOpenEnrollment: {"method":"POST","path":"/invitations/open/inspect","pathParams":[],"queryParams":[]},
	enrollOpenInvitation: {"method":"POST","path":"/invitations/open/enroll","pathParams":[],"queryParams":[]},
} as const;
