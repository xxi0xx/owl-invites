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
	"shareToken": string;
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
} as const;
