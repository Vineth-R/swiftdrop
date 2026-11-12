# SwiftDrop API Documentation (v1.1, consolidated)

> Converted from the supplied PDF. Text and table alignment have been preserved where possible; review complex tables and diagrams against the original PDF.


<!-- PDF page 1 -->

Oct 21, 2026        ·



SwiftDrop Parcel Delivery and Courier Management System. Version 1.1, October 2026.



This single document specifies every REST endpoint, WebSocket channel, webhook

payload and RabbitMQ event in SwiftDrop. It replaces the earlier version 1.0 and its two

add-on parts (the purpose appendix and the password-reset addendum): everything is

now merged into the section where it belongs.





## What changed from version 1.0



  Change                                                  Where



  Forgot-password and reset-password endpoints            Section 2

  added in place as 2.16 and 2.17, with token rules and

  the end-to-end flow



  Each endpoint now carries a one-line purpose,           Sections 2 to 14

  merged from the appendix



  Rate limits and error codes extended for the reset      Sections 1.7, 1.9

  flow; missing codes added (423

  ACCOUNT_LOCKED)



  Endpoint summary updated to include the two new         Section 15

  endpoints



  Header names made consistent (X-Request-Id              Sections 1.4, 13

  everywhere)









## 1. Conventions



1.1 Base URL and versioning

All paths in this document are relative to /api/v1 unless they start with /ws or

 /internal .


<!-- PDF page 2 -->

Item                                        Value



  Public base URL (local)                      http://localhost:8080



  API base path                                /api/v1



  WebSocket path                               /ws



  Content type                                 application/json; charset=utf-8 (file uploads

                                              use multipart/form-data )



  Timestamps                                  ISO 8601 in UTC, for example 2026-11-

                                              02T09:30:00Z



  Identifiers                                 UUID v4 for internal IDs. Public parcel lookup uses

                                              the tracking number, for example SDK7M2Q9XP4



  Money                                       Decimal number in LKR, two decimal places, for

                                              example 1250.00







1.2 Authentication



  Client                                                           Mechanism



  Browser users (customer, courier,                                HttpOnly, SameSite cookies sd_access

  dispatcher, admin, merchant dashboard)                           (short-lived JWT) and sd_refresh (refresh

                                                                   token), set by the login and Google callback

                                                                   endpoints



  API tools such as Postman and automated                           Authorization: Bearer <accessToken> .

  tests                                                            Send header X-Token-Mode: body on login

                                                                   to receive the tokens in the response body



  Merchant server-to-server calls                                   X-API-Key: <key> on /merchant/orders

                                                                   endpoints



  Internal service calls                                            X-Internal-Token: <shared secret> on

                                                                    /internal endpoints (not exposed through

                                                                   Nginx)





JWT access token claims: sub (user ID), role , exp , iat , jti . Access token lifetime is

15 minutes by default ( ACCESS_TOKEN_TTL ). Refresh token lifetime is 7 days by default

( REFRESH_TOKEN_TTL ) and is rotated on every use. The Google OAuth state lifetime is 10

minutes by default ( OAUTH_STATE_TTL ). Password reset links expire after 30 minutes by

default ( PASSWORD_RESET_TOKEN_TTL ). All four values are validated at startup. Access-token expiry is carried in the JWT exp claim; refresh-token, reset-token and OAuth-state expiries are stored as TIMESTAMPTZ and checked by the service.


<!-- PDF page 3 -->

1.3 Roles



  Role                       Description



  Public                     No login required



  Any user                   Any authenticated user



  CUSTOMER                   Registered customer



  MERCHANT                   Approved merchant account



  COURIER                    Delivery courier



  DISPATCHER                 Dispatcher



  ADMIN                      Administrator (also satisfies every DISPATCHER

                             permission)





Object-level rules apply on top of roles. For example a customer can only read their own

parcels and a courier can only update parcels currently assigned to them.





1.4 Standard headers



  Header                                    Direction             Description



   X-Request-Id                             Request (optional),   Correlation ID. Generated by the

                                            Response              gateway if absent and echoed in

                                                                  meta.requestId



   Idempotency-Key                          Request (optional)    Used on POST /parcels and POST

                                                                  /merchant/orders to prevent duplicates



   X-RateLimit-                             Response              Remaining requests in the current

  Remaining                                                       window



   Retry-After                              Response              Seconds to wait after a 429







1.5 Success response envelope



   {

       "data": { "id": "b6c1...", "status": "CREATED" },

       "meta": { "requestId": "req_01HF...", "timestamp": "2026-11-02T09:30:00Z" }

   }





List endpoints return data as an array and add pagination:


<!-- PDF page 4 -->

{

       "data": [ { "id": "..." } ],

       "meta": {

           "requestId": "req_01HF...",

           "timestamp": "2026-11-02T09:30:00Z",

           "pagination": { "page": 1, "pageSize": 20, "totalItems": 57,

   "totalPages": 3 }

       }

   }





Pagination query parameters on every list endpoint: page (default 1) and pageSize

(default 20, maximum 100). Sorting uses sort , such as createdAt:desc .





1.6 Error response envelope



   {

       "error": {

         "code": "VALIDATION_ERROR",

           "message": "One or more fields are invalid.",

           "details": [ { "field": "weightKg", "issue": "must be greater than 0" } ]

       },

       "meta": { "requestId": "req_01HF..." }

   }





Error messages never contain stack traces, SQL, secrets or personal data. details is

present only for validation errors.


<!-- PDF page 5 -->

1.7 Rate limits



  Endpoint group                                           Limit



   /auth/login , /auth/register                            10 requests per minute per IP



   /auth/forgot-password , /auth/reset-                    Rate limited per IP and, for forgot-

  password                                                 password, per submitted email. The

                                                           addendum gave no figure; the

                                                           recommended starting value is 5 requests

                                                           per minute per IP



   /users/me/password                                      5 requests per minute per signed-in user

   /tracking/{trackingNumber}                              30 requests per minute per IP



   /merchant/orders                                        120 requests per minute per API key



   /ai/chat                                                15 requests per minute per user



  All other endpoints                                      300 requests per minute per user







1.8 Data types used in this document



  Type                             Values



  Role                             CUSTOMER, MERCHANT, COURIER, DISPATCHER,

                                   ADMIN



  ParcelStatus                     CREATED, PICKED_UP, IN_TRANSIT,

                                   OUT_FOR_DELIVERY, DELIVERED, FAILED



  SizeCategory                     SMALL, MEDIUM, LARGE, OVERSIZED



  MerchantStatus                   PENDING, APPROVED, REJECTED, SUSPENDED







1.9 HTTP status and error codes



  HTTP          Code                                        Meaning



  400           BAD_REQUEST                                 Malformed JSON or missing parameter



  400           INVALID_OR_EXPIRED_RESET_TOKEN              Password-reset token is invalid, expired

                                                            or already used (2.17)



  401           UNAUTHENTICATED                             Missing or invalid credentials



  401           TOKEN_EXPIRED                               Access token expired. Call refresh


<!-- PDF page 6 -->

HTTP          Code                               Meaning



  403           FORBIDDEN                          Authenticated but not allowed (role or

                                                   ownership)



  404           NOT_FOUND                          Resource missing, or hidden because the

                                                   caller does not own it



  409           CONFLICT                           Duplicate or state conflict



  409           INVALID_TRANSITION                 Parcel status change not allowed



  413           PAYLOAD_TOO_LARGE                  Body or file above the limit



  415           UNSUPPORTED_MEDIA_TYPE             Wrong file type



  422           VALIDATION_ERROR                   One or more fields failed validation



  423           ACCOUNT_LOCKED                     Too many failed logins (2.2)



  429           RATE_LIMITED                       Too many requests



  500           INTERNAL_ERROR                     Unexpected server failure



  502           UPSTREAM_ERROR                     A dependency (Google, AI provider)

                                                   failed



  503           SERVICE_UNAVAILABLE                A dependent service timed out









## 2. Identity and User Service

Routed by Nginx from /api/v1/auth , /api/v1/users , /api/v1/admin/users and

 /api/v1/couriers .





2.1 POST /auth/register

Access: Public.



Purpose: Creates a new customer account. Returns the created profile; rejects duplicate

emails and invalid data.



Request body:


<!-- PDF page 7 -->

{

       "email": "nimal@example.com",

       "password": "Str0ngPass!",

       "fullName": "Nimal Perera",

       "phone": "+94771234567"

   }





Rules: valid email, password at least 8 characters with upper case, lower case and a digit,

 fullName 2 to 150 characters, phone in E.164 format.



Response 201:



   {

       "data": {

         "id": "3f1c0a52-8c0e-4e47-9d5e-1b6d7a9c2f10",

         "email": "nimal@example.com",

           "fullName": "Nimal Perera",

           "phone": "+94771234567",

           "role": "CUSTOMER",

           "createdAt": "2026-11-02T09:30:00Z"

       },

       "meta": { "requestId": "req_01", "timestamp": "2026-11-02T09:30:00Z" }

   }





Errors: 409 EMAIL_ALREADY_EXISTS , 422 VALIDATION_ERROR , 429.





2.2 POST /auth/login

Access: Public.

Unverified CUSTOMER accounts receive 403 EMAIL_VERIFICATION_REQUIRED and no
cookies. Inactive MERCHANT accounts, or merchants whose application is not
APPROVED, receive 403 MERCHANT_ACCOUNT_INACTIVE. Identity checks the Merchant
service status during merchant login and refresh; if it is unavailable the login
fails closed with 503. Access tokens carry an `sv` session version that must
match the current Identity session version.

403 MERCHANT_ACCOUNT_INACTIVE is answered only after the correct password. Its
`error.details` then say where the application stands, as `field` and `issue`
pairs: `status` (PENDING, REJECTED or SUSPENDED), `reason` (the administrator's
text, left out when there is none) and `supportEmail` (left out when none is
configured). The sign-in screen shows them to the owner. There are no details
when an administrator switched the account off, or when the status could not be
read for an already inactive account. Google sign-in applies the same approval
check as this endpoint.



Purpose: Authenticates a user with email and password and starts the session using

cookies. Optionally returns tokens in the body for API testing.



Request body:



   { "email": "nimal@example.com", "password": "Str0ngPass!" }





Response 200. Sets cookies sd_access and sd_refresh . With header X-Token-Mode:

body the tokens are also returned ( accessToken and refreshToken appear only in token

mode).


<!-- PDF page 8 -->

{

     "data": {

       "user": { "id": "3f1c...", "email": "nimal@example.com", "fullName":

   "Nimal Perera", "role": "CUSTOMER" },

           "expiresIn": 900,

           "accessToken": "eyJhbGciOi...",

         "refreshToken": "rt_9fA..."

       },

       "meta": { "requestId": "req_02", "timestamp": "2026-11-02T09:31:00Z" }

   }





Errors: 401 INVALID_CREDENTIALS (same message for unknown email and wrong

password), 403 ACCOUNT_DISABLED , 423 ACCOUNT_LOCKED (after repeated failures), 429.





2.3 POST /auth/refresh

Access: Public (requires a valid refresh token). Purpose: Exchanges a refresh token for a

new access token and rotates the refresh token.



Request: no body when using the sd_refresh cookie. In token mode send {

"refreshToken": "rt_9fA..." } .



Response 200 (tokens appear in the body only in token mode):



   { "data": { "expiresIn": 900, "accessToken": "eyJhbGciOi...", "refreshToken":

   "rt_Qw3..." }, "meta": { "requestId": "req_03", "timestamp": "2026-11-

   02T09:45:00Z" } }





Errors: 401 REFRESH_TOKEN_INVALID (expired, revoked or already used).

Reuse detection: a refresh token is single use. If a token that was already

rotated is presented again more than 10 seconds after its rotation, the service

treats it as stolen and revokes every token that descends from it, which ends

that session. The response is the same 401 REFRESH_TOKEN_INVALID. Other logins of

the same user are not affected. A repeat inside the 10 seconds (two tabs, or a

retry after a lost response) is refused without revoking anything.





2.4 POST /auth/logout

Access: Any user.



Purpose: Revokes the current refresh token and clears the cookies, ending the session.



Request: no body. Response 204 with no content.





2.5 GET /auth/google/login

Access: Public.



Purpose: Starts the Google OAuth 2.0 authorization code flow.



Query parameters: redirectTo (optional, relative frontend path such as /dashboard ).


<!-- PDF page 9 -->

Response 302 redirect to https://accounts.google.com/o/oauth2/v2/auth with

 client_id , redirect_uri , response_type=code , scope=openid email profile and a

random state . The response also sets a short-lived HttpOnly browser-binding cookie

( sd_oauth_binding_<state prefix> , SameSite=Lax, Path=/api/v1/auth/google, the configured

 OAUTH_STATE_TTL , default 10 minutes, Secure in production) that the callback must

present.





2.6 GET /auth/google/callback

Access: Public.



Purpose: Handles Google's redirect after consent and signs the user in.



Query parameters: code and state (both required). On error Google sends error . The

request must also send the browser-binding cookie set by 2.5.



Behaviour: validates the state and browser binding, exchanges the code, verifies the ID

token, finds or creates a CUSTOMER user (matching on google_sub , then email), sets the

 sd_access and sd_refresh cookies. If the matched account's email was not verified, the

link clears its existing password and revokes its refresh tokens; a password reset restores

password login. A missing or mismatched browser binding is rejected without consuming the

state.



Response 302 redirect to the frontend ( redirectTo or /dashboard ). On failure 302 to

 /login?error=oauth_failed .



Errors (as JSON when called directly): 400 OAUTH_STATE_INVALID , 502

 OAUTH_PROVIDER_ERROR .





2.7 GET /users/me

Access: Any user.



Purpose: Returns the signed-in user's profile, including role and authentication provider.



Response 200:



   {

       "data": {

           "id": "3f1c...",

           "email": "nimal@example.com",

           "fullName": "Nimal Perera",

           "phone": "+94771234567",

           "role": "CUSTOMER",

           "authProvider": "LOCAL",

           "createdAt": "2026-11-02T09:30:00Z"

       },

       "meta": { "requestId": "req_04", "timestamp": "2026-11-02T09:32:00Z" }

   }







2.8 PATCH /users/me

Access: Any user.



Purpose: Updates the user's own profile fields (full name, phone).


<!-- PDF page 10 -->

Request body (any subset): { "fullName": "Nimal P. Perera", "phone":

"+94771234000" }



Response 200: the updated user object (same shape as 2.7). Errors: 422.





2.9 PUT /users/me/password

Access: Any user (LOCAL accounts).



Purpose: Changes the signed-in user's own password. For users who forgot their

password use 2.16 and 2.17.



Request body: { "currentPassword": "Str0ngPass!", "newPassword":

"N3wStrongPass!" }



Response 204. All refresh tokens of the user are revoked. Errors: 401

 INVALID_CREDENTIALS (wrong current password), 409 PASSWORD_NOT_SET (Google-only

account), 422.





2.10 GET /admin/users

Access: ADMIN.



Purpose: Paginated, filterable list of users for account management.



Query: role , isActive (true or false), q (search name or email), page , pageSize ,

 sort .



Response 200:



   {

       "data": [

         { "id": "3f1c...", "email": "nimal@example.com", "fullName": "Nimal

   Perera", "phone": "+94771234567", "role": "CUSTOMER", "isActive": true,

   "createdAt": "2026-11-02T09:30:00Z" }

       ],

       "meta": { "requestId": "req_05", "timestamp": "2026-11-02T09:33:00Z",

       "pagination": { "page": 1, "pageSize": 20, "totalItems": 1, "totalPages":

   1 } }

   }







2.11 POST /admin/users

Access: ADMIN.



Purpose: Creates a staff account (courier, dispatcher or admin) with an assigned role.



Request body:


<!-- PDF page 11 -->

{

       "email": "kasun.courier@swiftdrop.lk",

       "password": "TempPass#2026",

       "fullName": "Kasun Silva",

       "phone": "+94712223344",

       "role": "COURIER",

       "courierProfile": { "vehicleType": "BIKE", "vehicleNumber": "BCD-4521",

   "homeZoneId": "7a1e..." }

   }





 courierProfile is required only when role is COURIER.



Response 201: the created user (as in 2.10) including courierProfile for couriers. Errors:

409 EMAIL_ALREADY_EXISTS , 422. The action is written to the audit log.





2.12 GET /admin/users/{userId}

Access: ADMIN. Purpose: Full administrative view of one user.



Response 200: one user object including courierProfile (if a courier), lastLoginAt

and updatedAt . Errors: 404.





2.13 PATCH /admin/users/{userId}

Access: ADMIN.



Purpose: Updates a user's profile or administrative fields.



Request body (any subset): fullName , phone , role , courierProfile ( vehicleType ,

 vehicleNumber , homeZoneId , isAvailable ).



Response 200: the updated user. Errors: 404, 422. Audited.





2.14 PATCH /admin/users/{userId}/status

Access: ADMIN.



Purpose: Activates or deactivates a user without editing the rest of the profile.

Deactivation revokes all refresh tokens.



Request body: { "isActive": false }



Response 200: { "data": { "id": "...", "isActive": false } } . Errors: 404, 409

 CANNOT_DEACTIVATE_SELF . Audited.





2.15 GET /couriers

Access: DISPATCHER, ADMIN.


<!-- PDF page 12 -->

Purpose: Lists active couriers for assignment.



Query: isAvailable (optional), zoneId (optional), page , pageSize .



Response 200:



   {

       "data": [

           { "id": "c1d2...", "fullName": "Kasun Silva", "phone": "+94712223344",

   "vehicleType": "BIKE", "homeZoneId": "7a1e...", "isAvailable": true }

       ],

       "meta": { "requestId": "req_06", "timestamp": "2026-11-02T09:34:00Z",

           "pagination": { "page": 1, "pageSize": 20, "totalItems": 1, "totalPages":

   1 } }

   }







2.16 POST /auth/forgot-password

Access: Public (rate limited).



Purpose: Starts the password-reset flow from the login page. The user enters their email;

SwiftDrop generates a cryptographically secure, short-lived, single-use reset token and

emails a reset link.



Request body:



   { "email": "nimal@example.com" }





Behaviour:



## 1. Validate the email field and apply rate limits.

## 2. If an account is eligible for password reset, create a random, high-entropy token with a

       short expiry ( PASSWORD_RESET_TOKEN_TTL , default 30 minutes) and store only a hash of

       the token with the user ID and expiry.

## 3. Email a link such as https://swiftdrop.example/reset-password?token=<token> to

       the submitted address.

## 4. Return the same public response whether or not the email belongs to an account, so

       registered addresses are not revealed. Never include the token in the API response or

       logs.



Response 202:



   {

       "data": { "message": "If an account exists for this email, a password reset

   link will be sent." },

     "meta": { "requestId": "req_41", "timestamp": "2026-11-05T09:10:00Z" }

   }


<!-- PDF page 13 -->

Errors: 422 VALIDATION_ERROR , 429 RATE_LIMITED , 500 INTERNAL_ERROR . There is no

distinct response for an unknown email.





2.17 POST /auth/reset-password

Access: Public (requires a valid reset token).



Purpose: Sets a new password after the user opens the emailed link. The frontend reads

the token from the URL query parameter and submits it with the new password, so the

user never types the token.



Request body:



   { "token": "<token-from-reset-link>", "newPassword": "N3wStr0ngPass!" }





Behaviour:



## 1. The reset page extracts the token query parameter and submits it with the new

       password.

## 2. Validate the token hash, expiry and unused status. Reject missing, invalid, expired or

       previously used tokens.

## 3. Validate the new password with the same strength rules as registration.

## 4. In one database transaction, update the password hash and mark the reset token as

    used. Invalidate existing refresh tokens and sessions so the old credentials stop

       working.

## 5. Never log or expose the raw reset token. After success, show a confirmation and send

       the user to log in.



Response 200:



   {

       "data": { "message": "Password has been reset successfully." },

       "meta": { "requestId": "req_42", "timestamp": "2026-11-05T09:15:00Z" }

   }





Errors: 400 INVALID_OR_EXPIRED_RESET_TOKEN , 422 VALIDATION_ERROR , 429

 RATE_LIMITED . Invalid, expired and already-used tokens all return the same generic

message.



Reset-link and token rules



       Generate tokens with a cryptographically secure random generator. Tokens are single-

       use and expire after a short period ( PASSWORD_RESET_TOKEN_TTL , default 30 minutes).

       Store a hash of each token, never the raw token. The raw token exists only in the

       emailed link.


<!-- PDF page 14 -->

Build links from a configured, trusted frontend base URL, never from an untrusted

       Host header.

       Use HTTPS in deployed environments. Rate limit both endpoints and never reveal

       whether an email has an account.

       The email provider is an implementation dependency. Responses must not disclose

       provider credentials or reset tokens.



End-to-end reset flow



## 1. The user selects Forgot password on the login page and submits their email.

## 2. The frontend calls POST /api/v1/auth/forgot-password .

## 3. The Identity Service generates a token, stores its hash and expiry, and emails the reset

       URL.

## 4. The user opens the link. The reset page reads the token from the URL and asks only for

       a new password.

## 5. The frontend calls POST /api/v1/auth/reset-password with token and

       newPassword .

## 6. The Identity Service validates and consumes the token, updates the password,

       invalidates existing sessions and confirms success.






2.18 POST /auth/verify-email

Access: Public.

Purpose: Consumes a single-use email verification token for a CUSTOMER account.
Only its SHA-256 hash is persisted. Unknown, expired and already-used tokens
return the same 400 response. The account is marked verified and the token is
consumed in one transaction.

Request body: { "token": "verification-token" }

Response 200: { "data": { "message": "Your email has been verified. You can now sign in to SwiftDrop." }, "meta": { "requestId": "req_20" } }

Errors: 400 INVALID_OR_EXPIRED_VERIFICATION_TOKEN, 415, 429.

2.19 POST /auth/resend-verification

Access: Public.

Purpose: Requests another verification email. The reply is always 202 for a
well-formed email so account existence is not revealed. Only an active,
unverified LOCAL CUSTOMER receives a new token; issuing one invalidates any
previous unused token. Rate limited per IP and per normalized email.

Request body: { "email": "nimal@example.com" }

Response 202: { "data": { "message": "If an account needs verification, a new verification email will be sent." }, "meta": { "requestId": "req_21" } }

Errors: 422, 429.

## 3. Pricing and Zones Service

Routed by Nginx from /api/v1/pricing , /api/v1/zones , /api/v1/admin/pricing and

 /api/v1/admin/zones .



Fee formula: total = base fee for (origin zone, destination zone) + weight band charge +

size surcharge.





3.1 POST /pricing/estimate

Access: Public (rate limited). The Parcel service recalculates the fee on creation.



Purpose: Estimates the delivery fee for a zone pair, weight and size before a parcel is

created.



Request body:



   {

       "originZoneId": "7a1e0c10-2b3d-4c5e-8f90-a1b2c3d4e5f6",

       "destinationZoneId": "9b2f1d21-3c4e-4d6f-9a01-b2c3d4e5f607",

       "weightKg": 2.0,

       "sizeCategory": "SMALL"

   }


<!-- PDF page 15 -->

Response 200:



   {

       "data": {

         "originZoneId": "7a1e...",

           "destinationZoneId": "9b2f...",

           "weightKg": 2.0,

           "sizeCategory": "SMALL",

           "currency": "LKR",

           "baseFee": 450.00,

           "weightCharge": 100.00,

           "sizeSurcharge": 0.00,

           "total": 550.00

       },

       "meta": { "requestId": "req_07", "timestamp": "2026-11-02T09:35:00Z" }

   }





Errors: 404 ZONE_NOT_FOUND , 422 VALIDATION_ERROR (weight must be above 0 and at

most the maximum band), 422 ROUTE_NOT_SERVICEABLE (no active pricing rule for the

zone pair), 422 WEIGHT_NOT_PRICED (no weight band covers the weight).





3.2 GET /zones

Access: Public.



Purpose: Lists active zones for pickup and destination dropdowns.



Response 200:



   {

       "data": [

         { "id": "7a1e...", "code": "CMB", "name": "Colombo", "districts":

   ["Colombo"] },

       { "id": "9b2f...", "code": "KDY", "name": "Kandy", "districts": ["Kandy",

   "Matale"] }

       ],

       "meta": { "requestId": "req_08", "timestamp": "2026-11-02T09:36:00Z" }

   }







3.3 POST /admin/zones

Access: ADMIN. Purpose: Creates a delivery zone that pricing and parcel booking can

use.



Request body: { "code": "GLE", "name": "Galle", "districts": ["Galle",

"Matara"] }


<!-- PDF page 16 -->

Response 201: { "data": { "id": "...", "code": "GLE", "name": "Galle",

"districts": ["Galle","Matara"], "isActive": true } } . Errors: 409

 ZONE_CODE_EXISTS , 422. Audited.





3.4 PATCH /admin/zones/{zoneId}

Access: ADMIN.



Purpose: Updates a zone's name, districts or active state.



Request body (any subset): name , districts , isActive . Response 200: the updated

zone. Errors: 404, 422.





3.5 GET /admin/pricing/rules

Access: ADMIN. Purpose: Lists base-fee pricing rules per zone pair.



Query: originZoneId , destinationZoneId , isActive , page , pageSize .



Response 200:



   {

       "data": [

       { "id": "r1...", "originZoneId": "7a1e...", "destinationZoneId":

   "9b2f...", "baseFee": 450.00, "currency": "LKR", "isActive": true,

   "updatedAt": "2026-10-20T08:00:00Z" }

       ],

       "meta": { "requestId": "req_09", "timestamp": "2026-11-02T09:37:00Z",

           "pagination": { "page": 1, "pageSize": 20, "totalItems": 1, "totalPages":

   1 } }

   }







3.6 POST /admin/pricing/rules

Access: ADMIN.



Purpose: Creates a base-fee rule for a zone pair.



Request body: { "originZoneId": "7a1e...", "destinationZoneId": "9b2f...",

"baseFee": 450.00 }



Response 201: the created rule. Errors: 404 ZONE_NOT_FOUND , 409 RULE_EXISTS (an

active rule already exists for the pair), 422. Audited.





3.7 PATCH /admin/pricing/rules/{ruleId}

Access: ADMIN.


<!-- PDF page 17 -->

Purpose: Updates a rule's base fee or active state. Changes never alter fees already

stored on existing parcels.



Request body (any subset): baseFee , isActive . Response 200: the updated rule. Errors:

404, 422. Audited.





3.8 GET /admin/pricing/bands

Access: ADMIN.



Purpose: Lists the weight bands used for the weight charge.



Response 200: { "data": [ { "id": "b1...", "minWeightKg": 0, "maxWeightKg": 1,

"charge": 0.00, "isActive": true }, { "id": "b2...", "minWeightKg": 1,

"maxWeightKg": 5, "charge": 100.00, "isActive": true } ] }





3.9 POST /admin/pricing/bands

Access: ADMIN.



Purpose: Creates a weight band with its charge.



Request body: { "minWeightKg": 5, "maxWeightKg": 10, "charge": 250.00 }



Response 201: the created band. Errors: 409 BAND_OVERLAP , 422 (max must exceed min).

Audited.





3.10 PATCH /admin/pricing/bands/{bandId}

Access: ADMIN.



Purpose: Updates a band's range, charge or active state.



Request body (any subset): minWeightKg , maxWeightKg , charge , isActive . Response

200: the updated band. Errors: 404, 409 BAND_OVERLAP , 422.





3.11 GET /admin/pricing/surcharges

Access: ADMIN.



Purpose: Lists the surcharge per size category.



Response 200: { "data": [ { "sizeCategory": "SMALL", "surcharge": 0.00 }, {

"sizeCategory": "MEDIUM", "surcharge": 150.00 }, { "sizeCategory": "LARGE",

"surcharge": 400.00 }, { "sizeCategory": "OVERSIZED", "surcharge": 900.00 } ]

}


<!-- PDF page 18 -->

3.12 PUT /admin/pricing/surcharges/{sizeCategory}

Access: ADMIN.



Purpose: Sets the surcharge for one size category.



Path: sizeCategory is one of SMALL, MEDIUM, LARGE, OVERSIZED. Request body: {

"surcharge": 175.00 }



Response 200: { "data": { "sizeCategory": "MEDIUM", "surcharge": 175.00 } } .

Errors: 422. Audited.





3.13 GET /admin/zones

Access: ADMIN.

Purpose: Lists all zones for the admin screen, including inactive ones, which GET /zones

leaves out.

Query: isActive , page , pageSize , sort . originZoneId and destinationZoneId are not

accepted and return 422. createdAt and updatedAt are sort keys only and are not

returned in the response.

Response 200: { "data": [ { "id": "7a1e...", "code": "CMB", "name": "Colombo",

"districts": ["Colombo"], "isActive": true } ], "meta": { "requestId": "req_15",

"timestamp": "2026-11-02T09:38:00Z", "pagination": { "page": 1, "pageSize": 20,

"totalItems": 1, "totalPages": 1 } } } . Errors: 401, 403, 422.





## 4. Parcel Operations Service: Customer and Public Endpoints

Routed by Nginx from /api/v1/parcels and /api/v1/tracking .





4.1 POST /parcels

Access: CUSTOMER.



Purpose: Creates a delivery request. The server recalculates the fee through the Pricing

service, stores the price on the parcel, generates a tracking number and records the first

status.



Optional header: Idempotency-Key: <uuid> .

Implemented replay semantics: the key is scoped to the authenticated customer.
An identical retry returns the existing parcel with 201, including after a
concurrent unique-index conflict; pricing is not required for an existing replay.
Reusing a key for different sender/recipient, weight, size or description returns
409 `IDEMPOTENCY_CONFLICT`. `expectedFee` is consent metadata and is excluded from
the successful-replay comparison. A request rejected with `PRICE_CHANGED` has not
created a parcel; retain the key and explicitly reconfirm the current fee.



Request body:



   {

       "sender": {

           "name": "Nimal Perera",

           "phone": "+94771234567",

           "address": "12 Temple Road, Nugegoda",

           "city": "Colombo",

           "zoneId": "7a1e0c10-2b3d-4c5e-8f90-a1b2c3d4e5f6"

       },

       "recipient": {

           "name": "Amali Fernando",

           "phone": "+94712345678",

           "address": "45 Lake Drive",

           "city": "Kandy",

           "zoneId": "9b2f1d21-3c4e-4d6f-9a01-b2c3d4e5f607"

       },

       "weightKg": 2.0,


<!-- PDF page 19 -->

"sizeCategory": "SMALL",

       "description": "Books",

       "expectedFee": 550.00

   }





 expectedFee is the fee the customer saw in the estimate. If the current fee differs, the

request is rejected so the customer can confirm the new price.



Response 201:



   {

       "data": {

           "id": "e5a1b7c2-6d3f-4a8b-9c10-2d3e4f5a6b7c",

           "trackingNumber": "SDK7M2Q9XP4",

           "status": "CREATED",

           "sender": { "name": "Nimal Perera", "city": "Colombo" },

           "recipient": { "name": "Amali Fernando", "city": "Kandy" },

           "weightKg": 2.0,

           "sizeCategory": "SMALL",

           "description": "Books",

           "fee": { "currency": "LKR", "baseFee": 450.00, "weightCharge": 100.00,

   "sizeSurcharge": 0.00, "total": 550.00 },

           "createdAt": "2026-11-02T10:00:00Z"

       },

       "meta": { "requestId": "req_10", "timestamp": "2026-11-02T10:00:00Z" }

   }





Errors: 409 PRICE_CHANGED (response includes currentFee ), 422 VALIDATION_ERROR ,

422 ROUTE_NOT_SERVICEABLE , 503 SERVICE_UNAVAILABLE (pricing unreachable).

`PRICE_CHANGED` uses the common error envelope with numeric
`error.currentFee` (the current total in LKR). Unknown JSON fields or invalid
decimal encodings return 400; semantic field violations return 422. Request
bodies are limited to 1 MiB. Money and weight accept at most two decimal places.



Side effects: inserts the parcel, the first status history row and an outbox event, then

queues `parcel.created` for M6's relay to publish as `parcel.created.v1`.
The creation transaction is implemented; the merged M6 relay publishes after
commit. A successful creation does not claim email delivery.





4.2 GET /parcels

Access: CUSTOMER.



Purpose: Lists the signed-in customer's parcels (delivery history).



Query: status , from and to (ISO dates), page , pageSize , sort (default

 createdAt:desc ).

Implemented bounds: `page` is 1–100000, `pageSize` is 1–100 (default 20), and
`sort` is `createdAt:asc` or `createdAt:desc`. Both date boundaries are inclusive
UTC calendar dates. Tied creation timestamps use the parcel ID for stable order.
The query always includes the authenticated customer's owner ID.



Response 200:


<!-- PDF page 20 -->

{

       "data": [

           {

               "id": "e5a1...",

               "trackingNumber": "SDK7M2Q9XP4",

               "status": "IN_TRANSIT",

               "recipientName": "Amali Fernando",

               "dropoffCity": "Kandy",

               "total": 550.00,

               "currency": "LKR",

               "createdAt": "2026-11-02T10:00:00Z"

           }

       ],

       "meta": { "requestId": "req_11", "timestamp": "2026-11-02T10:05:00Z",

           "pagination": { "page": 1, "pageSize": 20, "totalItems": 1, "totalPages":

   1 } }

   }







4.3 GET /parcels/{parcelId}

Access: CUSTOMER (owner), DISPATCHER, ADMIN.



Purpose: Full details of one parcel.



Response 200:


<!-- PDF page 21 -->

{

       "data": {

         "id": "e5a1...",

           "trackingNumber": "SDK7M2Q9XP4",

           "status": "IN_TRANSIT",

       "sender": { "name": "Nimal Perera", "phone": "+94771234567", "address":

   "12 Temple Road, Nugegoda", "city": "Colombo", "zoneId": "7a1e..." },

           "recipient": { "name": "Amali Fernando", "phone": "+94712345678",

   "address": "45 Lake Drive", "city": "Kandy", "zoneId": "9b2f..." },

       "weightKg": 2.0,

           "sizeCategory": "SMALL",

           "description": "Books",

           "fee": { "currency": "LKR", "baseFee": 450.00, "weightCharge": 100.00,

   "sizeSurcharge": 0.00, "total": 550.00 },

           "courier": { "fullName": "Kasun Silva", "phone": "+94712223344" },

           "failureReason": null,

           "createdAt": "2026-11-02T10:00:00Z",

           "updatedAt": "2026-11-02T13:10:00Z",

         "deliveredAt": null

       },

       "meta": { "requestId": "req_12", "timestamp": "2026-11-02T13:11:00Z" }

   }





 courier is null until assigned. Customers receive the assigned courier's fullName and

phone; dispatchers and admins also receive courier.id . If the assigned courier user no

longer exists, the parcel detail is still returned with courier null (the parcel is not

made unavailable); a genuine Identity outage returns 503 SERVICE_UNAVAILABLE . Errors:

404 (also returned when a customer asks for a parcel they do not own, so IDs cannot be

probed). State-changing parcel routes require Content-Type: application/json ; session

cookies are HttpOnly and SameSite=Lax.





4.4 GET /parcels/{parcelId}/history

Access: CUSTOMER (owner), DISPATCHER, ADMIN.



Purpose: Status history, oldest first, with previous and current status, notes, actor role

and timestamps.



Response 200:


<!-- PDF page 22 -->

{

       "data": [

           { "previousStatus": null, "status": "CREATED", "note": null,

   "failureReason": null, "actorRole": "CUSTOMER", "changedAt": "2026-11-

   02T10:00:00Z" },

       { "previousStatus": "CREATED", "status": "PICKED_UP", "note": "Collected

   from sender", "failureReason": null, "actorRole": "COURIER", "changedAt":

   "2026-11-02T11:20:00Z" }

     ],

       "meta": { "requestId": "req_13", "timestamp": "2026-11-02T13:12:00Z" }

   }





Errors: 404.





4.5 GET /parcels/{parcelId}/receipt

Access: CUSTOMER (owner), ADMIN.



Purpose: Downloads the delivery receipt as a PDF.



Response 200:



   Content-Type: application/pdf

   Content-Disposition: attachment; filename="receipt_SDK7M2Q9XP4.pdf"





The PDF contains the tracking number, sender and recipient names, cities, weight, size,

the stored fee breakdown, status and dates. Errors: 404.





4.6 GET /tracking/{trackingNumber}

Access: Public (rate limited).



Purpose: Public, mobile-friendly tracking lookup by tracking number. Returns only fields

designated public: no addresses, phone numbers, names or notes.



Response 200:


<!-- PDF page 23 -->

{

       "data": {

           "trackingNumber": "SDK7M2Q9XP4",

           "status": "OUT_FOR_DELIVERY",

           "pickupCity": "Colombo",

           "dropoffCity": "Kandy",

           "lastUpdatedAt": "2026-11-02T15:40:00Z",

           "timeline": [

               { "status": "CREATED", "at": "2026-11-02T10:00:00Z" },

               { "status": "PICKED_UP", "at": "2026-11-02T11:20:00Z" },

               { "status": "IN_TRANSIT", "at": "2026-11-02T13:10:00Z" },

               { "status": "OUT_FOR_DELIVERY", "at": "2026-11-02T15:40:00Z" }

         ]

       },

       "meta": { "requestId": "req_14", "timestamp": "2026-11-02T15:41:00Z" }

   }





Errors: 404 TRACKING_NOT_FOUND , 429.







## 5. Dispatch Endpoints

Provided by the Parcel Operations service. Routed from /api/v1/dispatch and

 /api/v1/admin/parcels .





5.1 GET /dispatch/unassigned

Access: DISPATCHER, ADMIN.



Purpose: Lists parcels that have no current courier, for dispatch planning.



Query: zoneId (pickup zone), from , to , page , pageSize , sort (default

 createdAt:asc ).



Response 200:



   {

       "data": [

           {

               "id": "e5a1...",

               "trackingNumber": "SDK7M2Q9XP4",

               "status": "CREATED",

               "pickupAddress": "12 Temple Road, Nugegoda",

               "pickupCity": "Colombo",

               "pickupZoneId": "7a1e...",

               "dropoffAddress": "45 Lake Drive",

               "dropoffCity": "Kandy",


<!-- PDF page 24 -->

"dropoffZoneId": "9b2f...",

               "weightKg": 2.0,

               "sizeCategory": "SMALL",

               "createdAt": "2026-11-02T10:00:00Z"

           }

       ],

       "meta": { "requestId": "req_15", "timestamp": "2026-11-02T10:10:00Z",

         "pagination": { "page": 1, "pageSize": 20, "totalItems": 1, "totalPages":

   1 } }

   }







5.2 PUT /dispatch/parcels/{parcelId}/assignment

Access: DISPATCHER, ADMIN.



Purpose: Assigns a parcel to a courier, or reassigns it to a different courier.



Request body:



   { "courierId": "c1d2e3f4-a5b6-4c7d-8e9f-0a1b2c3d4e5f", "reason": "Original

   courier unavailable" }





 reason is optional for a first assignment and recommended for a reassignment.



Response 200:



   {

       "data": {

         "parcelId": "e5a1...",

         "trackingNumber": "SDK7M2Q9XP4",

           "courierId": "c1d2...",

           "previousCourierId": null,

         "assignedBy": "d9e8...",

         "assignedAt": "2026-11-02T10:15:00Z"

       },

       "meta": { "requestId": "req_16", "timestamp": "2026-11-02T10:15:00Z" }

   }





Rules: the parcel must not be DELIVERED or FAILED. The courier must be active and

available (checked through the Identity service). Only one current assignment exists per

parcel.



Errors: 404 PARCEL_NOT_FOUND , 409 PARCEL_CLOSED , 422 COURIER_NOT_ELIGIBLE , 503.

Side effect: publishes parcel.assigned.v1 .


<!-- PDF page 25 -->

5.3 GET /dispatch/workload

Access: DISPATCHER, ADMIN.



Purpose: Shows each courier's availability and active deliveries so dispatchers can

balance assignments.



Response 200:



   {

       "data": [

           {

               "courierId": "c1d2...",

               "fullName": "Kasun Silva",

               "isAvailable": true,

               "activeDeliveries": 4,

         "byStatus": { "CREATED": 1, "PICKED_UP": 1, "IN_TRANSIT": 1,

   "OUT_FOR_DELIVERY": 1 }

       }

       ],

       "meta": { "requestId": "req_17", "timestamp": "2026-11-02T10:20:00Z" }

   }





Active means assigned and not DELIVERED or FAILED.





5.4 GET /admin/parcels

Access: DISPATCHER, ADMIN.



Purpose: Searches every parcel in the system for monitoring.



Query: status , courierId , merchantId , customerId , trackingNumber , from , to ,

 page , pageSize , sort .



Response 200: a paginated list. Each item has id , trackingNumber , status ,

 pickupCity , dropoffCity , courierId , merchantId , customerId , total , createdAt .







## 6. Courier Endpoints

Provided by the Parcel Operations service. Routed from /api/v1/courier . Every

endpoint checks that the parcel is currently assigned to the calling courier.





6.1 GET /courier/deliveries

Access: COURIER.


<!-- PDF page 26 -->

Purpose: Lists the courier's deliveries for a day: active parcels plus parcels finished on the

selected date.



Query: date (default today, in the Asia/Colombo time zone), status , page , pageSize .



Response 200:



   {

       "data": [

         {

               "id": "e5a1...",

               "trackingNumber": "SDK7M2Q9XP4",

               "status": "PICKED_UP",

               "pickupAddress": "12 Temple Road, Nugegoda",

               "dropoffAddress": "45 Lake Drive",

               "dropoffCity": "Kandy",

               "recipientName": "Amali Fernando",

               "recipientPhone": "+94712345678",

               "weightKg": 2.0,

               "assignedAt": "2026-11-02T10:15:00Z"

           }

       ],

       "meta": { "requestId": "req_18", "timestamp": "2026-11-02T10:30:00Z",

         "pagination": { "page": 1, "pageSize": 20, "totalItems": 1, "totalPages":

   1 } }

   }







6.2 GET /courier/deliveries/{parcelId}

Access: COURIER (assigned only).



Purpose: Detailed delivery view for one assigned parcel, including contacts, history and

proof metadata.



Response 200: the item shape from 6.1 plus description , sizeCategory , senderName ,

 senderPhone , history (same shape as 4.4) and proof (null or { "recipientName",

"note", "hasPhoto", "createdAt" } ).



Errors: 403 FORBIDDEN when the parcel is not assigned to this courier, 404.





6.3 POST /courier/deliveries/{parcelId}/status

Access: COURIER (assigned only).



Purpose: Moves the parcel to its next status, validating the transition and the required

fields.



Request body:


<!-- PDF page 27 -->

{

       "status": "DELIVERED",

       "note": "Handed over at the front door",

       "recipientName": "Amali Fernando"

   }





For a failed delivery:



   { "status": "FAILED", "failureReason": "Recipient not available after two

   attempts" }





Allowed transitions:





  From                                      To



  CREATED                                   PICKED_UP, FAILED



  PICKED_UP                                 IN_TRANSIT, FAILED



  IN_TRANSIT                                OUT_FOR_DELIVERY, FAILED



  OUT_FOR_DELIVERY                          DELIVERED, FAILED



  DELIVERED                                 None (terminal)



  FAILED                                    None (terminal)





Field rules: failureReason is required (5 to 255 characters) when status is FAILED.

 recipientName is required when status is DELIVERED. note is optional (up to 500

characters).



Response 200:



   {

       "data": {

         "parcelId": "e5a1...",

           "trackingNumber": "SDK7M2Q9XP4",

           "previousStatus": "OUT_FOR_DELIVERY",

           "status": "DELIVERED",

         "updatedAt": "2026-11-02T16:05:00Z"

       },

       "meta": { "requestId": "req_19", "timestamp": "2026-11-02T16:05:00Z" }

   }





Errors: 403 FORBIDDEN , 404, 409 INVALID_TRANSITION , 422 FAILURE_REASON_REQUIRED ,

422 RECIPIENT_REQUIRED .


<!-- PDF page 28 -->

Side effects, in one database transaction: update the parcel status, insert the status

history row, insert the outbox event. After commit the event parcel.status_changed.v1 is

published to RabbitMQ, which triggers email notification, the merchant webhook and live

tracking updates.





6.4 POST /courier/deliveries/{parcelId}/proof

Access: COURIER (assigned only).



Purpose: Uploads proof of delivery: recipient name, a note and an optional photo.



Content type: multipart/form-data . At least one field must be present.





  Field                          Type         Required   Rules



  photo                          file         No         JPEG or PNG, at most 5 MB



  recipientName                  text         No         Up to 150 characters



  note                           text         No         Up to 500 characters





Response 201:



   {

     "data": { "id": "p1f2...", "parcelId": "e5a1...", "recipientName": "Amali

   Fernando", "note": "Left with security", "hasPhoto": true, "createdAt":

   "2026-11-02T16:04:00Z" },

     "meta": { "requestId": "req_20", "timestamp": "2026-11-02T16:04:00Z" }

   }





Errors: 403, 404, 413 PAYLOAD_TOO_LARGE , 415 UNSUPPORTED_MEDIA_TYPE . Files are saved

under a generated safe name and are never served from a public path.





## 7. Report Endpoints

Provided by the Parcel Operations service.





7.1 GET /reports/daily

Access: DISPATCHER, ADMIN.



Purpose: Daily counts of created, delivered and failed parcels.



Query: from and to (ISO dates, default the last 7 days, maximum range 90 days).



Response 200:


<!-- PDF page 29 -->

{

       "data": [

         { "date": "2026-11-01", "created": 42, "delivered": 35, "failed": 3 },

         { "date": "2026-11-02", "created": 51, "delivered": 44, "failed": 2 }

       ],

       "meta": { "requestId": "req_21", "timestamp": "2026-11-03T08:00:00Z" }

   }





Errors: 422 (range invalid).





7.2 GET /reports/outcomes

Access: DISPATCHER, ADMIN.



Purpose: Success and failure rates over a period, with the most common failure reasons.



Query: from , to (default the last 30 days).



Response 200:



   {

       "data": {

           "from": "2026-10-04",

           "to": "2026-11-03",

           "totals": { "created": 410, "delivered": 352, "failed": 21, "inProgress":

   37 },

       "successRate": 94.37,

       "failureRate": 5.63,

           "topFailureReasons": [

             { "reason": "Recipient not available", "count": 9 },

             { "reason": "Incorrect address", "count": 6 }

         ]

       },

       "meta": { "requestId": "req_22", "timestamp": "2026-11-03T08:01:00Z" }

   }





Rates are percentages of finished parcels (delivered plus failed).





## 8. Merchant Integration Service

Routed by Nginx from /api/v1/merchants , /api/v1/merchant and

 /api/v1/admin/merchants .





8.1 POST /merchants/register

Access: Public.


<!-- PDF page 30 -->

Purpose: Registers a merchant business and its owner account. The merchant stays

PENDING until an administrator approves it.

`business.registrationNumber` is required. It must be 3-50 letters, digits,
spaces, hyphens, slashes or dots, with at least one digit. The stored value is
trimmed, whitespace-collapsed and upper-cased. Missing, null, empty and
whitespace-only values are rejected with `422 VALIDATION_ERROR`.

Request body:



   {

        "email": "owner@shopzone.lk",

        "password": "Str0ngPass!",

        "fullName": "Ruwan Jayasuriya",

        "phone": "+94773334455",

        "business": {

            "name": "ShopZone Lanka",

            "registrationNumber": "PV 123456",

            "businessAddress": "88 Galle Road, Colombo 03",

            "contactEmail": "support@shopzone.lk",

            "contactPhone": "+94112223344",

            "pickupAddress": "88 Galle Road, Colombo 03",

            "pickupZoneId": "7a1e..."

        }

   }





Response 201:



   {

        "data": { "merchantId": "m1a2...", "userId": "u9b8...", "status": "PENDING"

   },

        "meta": { "requestId": "req_30", "timestamp": "2026-11-04T08:00:00Z" }

   }





Errors: 409 EMAIL_ALREADY_EXISTS , 422 VALIDATION_ERROR , 429.





8.2 GET /merchants/me

Access: MERCHANT.



Purpose: Returns the merchant's own business profile and approval status.



Response 200:


<!-- PDF page 31 -->

{

       "data": {

         "id": "m1a2...",

         "businessName": "ShopZone Lanka",

           "registrationNumber": "PV 123456",

           "contactEmail": "support@shopzone.lk",

           "contactPhone": "+94112223344",

           "pickupAddress": "88 Galle Road, Colombo 03",

           "pickupZoneId": "7a1e...",

           "status": "APPROVED",

         "approvedAt": "2026-11-04T09:00:00Z"

       },

       "meta": { "requestId": "req_31", "timestamp": "2026-11-04T09:05:00Z" }

   }







8.3 PATCH /merchants/me

Access: MERCHANT.



Purpose: Updates contact details and the default pickup address and zone.



Request body (any subset): contactEmail , contactPhone , pickupAddress ,

 pickupZoneId . Response 200: the updated profile (as 8.2). Errors: 422.





8.4 GET /admin/merchants

Access: ADMIN.



Purpose: Lists merchant applications and accounts.



Query: status (MerchantStatus), q (business name), page , pageSize , sort .



Response 200: a paginated list. Each item has id , businessName , contactEmail ,

 status , createdAt , approvedAt .





8.5 GET /admin/merchants/{merchantId}

Access: ADMIN.



Purpose: Full administrative view of one merchant.



Response 200: the full profile (as 8.2) plus ownerUserId , approvedBy , rejectionReason ,

 createdAt . Errors: 404.





8.6 POST /admin/merchants/{merchantId}/approve

Access: ADMIN.


<!-- PDF page 32 -->

Purpose: Approves a pending (or suspended) merchant so it can use merchant features

and API keys.



Request: no body. Response 200: { "data": { "id": "m1a2...", "status":

"APPROVED", "approvedAt": "2026-11-04T09:00:00Z" } } .



Errors: 404, 409 INVALID_MERCHANT_STATE (allowed from PENDING or SUSPENDED).

Audited.





8.7 POST /admin/merchants/{merchantId}/reject

Access: ADMIN.



Purpose: Rejects a pending application and optionally records a merchant-facing
reason. An empty or whitespace-only reason is stored as NULL and omitted from the
email.



Request body: { "reason": "Business registration could not be verified" }



Response 200: { "data": { "id": "m1a2...", "status": "REJECTED" } } . Errors:

404, 409 INVALID_MERCHANT_STATE (allowed from PENDING), 422 (reason longer than
1,000 characters).

Audited.





8.8 POST /admin/merchants/{merchantId}/suspend

Access: ADMIN.



Purpose: Suspends an approved merchant. Existing API keys are rejected while

suspended.



Request body: { "reason": "Repeated misuse of the API" }



Response 200: { "data": { "id": "m1a2...", "status": "SUSPENDED" } } . Errors:

404, 409 INVALID_MERCHANT_STATE , 422. Audited.





Merchant registration (8.1) creates an inactive Identity account and a PENDING
application. No session is issued. Only an ADMIN approval applies a lifecycle
transition, increments the Identity session version and activates the account.
Pending, rejected and suspended merchants cannot obtain sessions.

8.6 to 8.8 lifecycle rules:

- Every actual approval, rejection, suspension and reapproval is a new lifecycle
  transition with a unique transition ID and per-merchant sequence.
- Identity increments session_version for every newly applied transition, even
  when is_active already equals the target. Retries of the same transition do
  not increment it again.
- A reused transition ID with a different payload returns 409
  TRANSITION_ID_CONFLICT. A stale sequence returns 409 STALE_TRANSITION.
- Suspension commits SUSPENDED immediately, then deactivates Identity and
  revokes refresh tokens. If Identity is unavailable the response reports
  identitySync PENDING and the merchant stays blocked.
- Rejection reason is optional, nullable and limited to 1,000 characters. Empty
  or whitespace-only values are stored as NULL. The rejection email omits the
  reason when none is provided. Suspension still requires a reason.
- The administrator's list (8.4) and detail (8.5) carry `identitySync: "PENDING"`
  for a REJECTED or SUSPENDED merchant whose status has not yet reached Identity,
  and leave it out otherwise. Repeating the reject or suspend call retries the
  sync with the same transition. A retry does not email the merchant again, and
  one that finds the sync already applied answers `alreadyDone: true`.
- `/merchant/pending` is a public informational page and never shows merchant
  details or rejection reasons.

8.9 GET /merchants/me/keys

Access: MERCHANT.



Purpose: Lists API keys and their metadata. The secret part of a key is never returned

again.



Response 200:


<!-- PDF page 33 -->

{

       "data": [

       { "id": "k1...", "name": "Production", "keyPrefix": "sdk_live_ab12",

   "status": "ACTIVE", "lastUsedAt": "2026-11-04T12:00:00Z", "createdAt": "2026-

   11-04T09:10:00Z" }

       ],

       "meta": { "requestId": "req_32", "timestamp": "2026-11-04T12:30:00Z" }

   }







8.10 POST /merchants/me/keys

Access: MERCHANT (approved only).



Purpose: Creates an API key for server-to-server calls. The full key is shown once.



Request body: { "name": "Production" }



Response 201:



   {

       "data": {

           "id": "k1...",

           "name": "Production",

           "apiKey": "sdk_live_ab12cd34_N8x2Zq0pLr5TyUvW9kJ3mHgF",

           "keyPrefix": "sdk_live_ab12",

           "createdAt": "2026-11-04T09:10:00Z"

       },

       "meta": { "requestId": "req_33", "timestamp": "2026-11-04T09:10:00Z" }

   }





Errors: 403 MERCHANT_NOT_APPROVED , 409 KEY_LIMIT_REACHED (maximum 5 active keys),

422.





8.11 DELETE /merchants/me/keys/{keyId}

Access: MERCHANT.



Purpose: Revokes a key immediately.



Response 204. Errors: 404.





8.12 GET /merchants/me/webhook

Access: MERCHANT.



Purpose: Returns the webhook URL, subscribed event types and active state, without the

signing secret.


<!-- PDF page 34 -->

Response 200:



   {

     "data": { "url": "https://shopzone.lk/hooks/swiftdrop", "eventTypes":

   ["parcel.status_changed"], "isActive": true, "updatedAt": "2026-11-

   04T09:20:00Z" },

       "meta": { "requestId": "req_34", "timestamp": "2026-11-04T09:21:00Z" }

   }





Response 404 WEBHOOK_NOT_CONFIGURED if none exists.





8.13 PUT /merchants/me/webhook

Access: MERCHANT (approved only).



Purpose: Creates or replaces the webhook configuration, including optional secret

rotation.



Request body:



   {

       "url": "https://shopzone.lk/hooks/swiftdrop",

       "eventTypes": ["parcel.status_changed"],

       "isActive": true,

       "rotateSecret": false

   }





The URL must use HTTPS and must not point to a private or loopback address. A new

signing secret is generated on first save, or when rotateSecret is true.



Response 200 ( signingSecret appears only when newly generated):



   {

     "data": { "url": "https://shopzone.lk/hooks/swiftdrop", "eventTypes":

   ["parcel.status_changed"], "isActive": true, "signingSecret":

   "whsec_3F9aQ1xK7LmP2vR8" },

       "meta": { "requestId": "req_35", "timestamp": "2026-11-04T09:20:00Z" }

   }





Errors: 403 MERCHANT_NOT_APPROVED , 422 VALIDATION_ERROR , 422

 WEBHOOK_URL_NOT_ALLOWED .





8.14 POST /merchants/me/webhook/test

Access: MERCHANT.



Purpose: Sends a sample event to the configured URL, records the attempt in the
delivery log (8.15) and reports the result. The sample uses tracking number
`SDNTEST0001` and event id generated for that send. A failed test is recorded as
a dead-lettered attempt; it is not retried. The endpoint requires an APPROVED
merchant.


<!-- PDF page 35 -->

Request: no body. Response 200:



   { "data": { "delivered": true, "httpStatus": 200, "durationMs": 182 },

     "meta": { "requestId": "req_36", "timestamp": "2026-11-04T09:22:00Z" } }





Errors: 404 WEBHOOK_NOT_CONFIGURED . A failed delivery still returns 200 with delivered

false and the httpStatus (null on timeout).





8.15 GET /merchants/me/webhook/deliveries

Access: MERCHANT.



Purpose: Delivery attempt log, including retries and dead-letter status, for

troubleshooting.



Query: success (true or false), trackingNumber , page , pageSize .



Response 200:



   {

       "data": [

       { "eventId": "6c8f8e56-5c6b-4d7c-9d9a-5f0f1f2b3c4d", "trackingNumber": "SDK7M2Q9XP4", "attemptNumber":

   2, "httpStatus": 503, "success": false, "nextRetryAt": "2026-11-

   04T12:35:00Z", "deadLettered": false, "createdAt": "2026-11-04T12:30:00Z" }

       ],

       "meta": { "requestId": "req_37", "timestamp": "2026-11-04T12:31:00Z",

         "pagination": { "page": 1, "pageSize": 20, "totalItems": 1, "totalPages":

   1 } }

   }







8.16 POST /merchant/orders

Access: Merchant API key ( X-API-Key ), approved merchant only.



Purpose: Creates a delivery order from the merchant's e-commerce system. Duplicates

are handled idempotently.



Optional header: Idempotency-Key: <string up to 100 characters> .



Request body:


<!-- PDF page 36 -->

{

     "externalOrderId": "SZ-100245",

     "pickup": { "name": "ShopZone Warehouse", "phone": "+94112223344",

   "address": "88 Galle Road, Colombo 03", "city": "Colombo", "zoneId":

   "7a1e..." },

     "recipient": { "name": "Amali Fernando", "phone": "+94712345678",

   "address": "45 Lake Drive", "city": "Kandy", "zoneId": "9b2f..." },

       "weightKg": 1.5,

       "sizeCategory": "SMALL",

       "description": "Phone case"

   }





 pickup is optional. When omitted, the merchant's default pickup address and zone are

used.



Response 201:



   {

       "data": {

           "orderId": "o7c6...",

           "externalOrderId": "SZ-100245",

           "trackingNumber": "SDN4T8W2LC6",

       "status": "CREATED",

       "fee": { "currency": "LKR", "baseFee": 450.00, "weightCharge": 100.00,

   "sizeSurcharge": 0.00, "total": 550.00 },

         "createdAt": "2026-11-04T10:00:00Z"

       },

       "meta": { "requestId": "req_38", "timestamp": "2026-11-04T10:00:00Z" }

   }





Duplicate handling: the same externalOrderId (or Idempotency-Key ) with the same

payload returns 200 with the original order. The same externalOrderId with a different

payload returns 409 DUPLICATE_ORDER . No duplicate parcel is ever created.



Errors: 401 INVALID_API_KEY (missing, unknown or revoked), 403

 MERCHANT_NOT_APPROVED , 409 DUPLICATE_ORDER , 422 VALIDATION_ERROR , 422

 ROUTE_NOT_SERVICEABLE , 429, 503.





8.17 GET /merchant/orders

Access: MERCHANT (JWT) or Merchant API key.



Purpose: Dashboard list of the caller's own orders.



Query: status , from , to , externalOrderId , page , pageSize , sort .



Response 200:


<!-- PDF page 37 -->

{

       "data": [

         { "orderId": "o7c6...", "externalOrderId": "SZ-100245", "trackingNumber":

   "SDN4T8W2LC6", "status": "IN_TRANSIT", "recipientName": "Amali Fernando",

   "dropoffCity": "Kandy", "total": 550.00, "currency": "LKR", "createdAt":

   "2026-11-04T10:00:00Z", "updatedAt": "2026-11-04T13:00:00Z" }

       ],

       "meta": { "requestId": "req_39", "timestamp": "2026-11-04T13:05:00Z",

         "pagination": { "page": 1, "pageSize": 20, "totalItems": 1, "totalPages":

   1 } }

   }







8.18 GET /merchant/orders/{orderId}

Access: MERCHANT (JWT) or Merchant API key, own orders only.



Purpose: Details of one order, including fee and status timeline.



Response 200: the item shape from 8.17 plus weightKg , sizeCategory , description ,

 fee and timeline (status and timestamp pairs as in 4.6).



Errors: 404 (also returned for another merchant's order).





## 9. Real-Time Tracking (WebSocket)

Endpoint: ws://localhost:8080/ws ( wss:// when deployed behind TLS). Purpose:

Pushes live, public-safe parcel status changes to subscribed clients.



The realtime service is not the source of truth. After a page refresh or reconnect the

client fetches the current status through GET /tracking/{trackingNumber} or GET

/parcels/{parcelId} .





9.1 Connection and authentication



  Item                               Rule



  Authentication                     Browser cookies are sent automatically. Non-

                                     browser clients may add ?token=<accessToken> .

                                     Anonymous connections are allowed for public

                                     tracking only



  Heartbeat                          Server sends a ping every 30 seconds. Client must

                                     reply with a pong, or send { "type": "ping" }



  Subscription limit                 20 subscriptions per connection


<!-- PDF page 38 -->

Item                               Rule



  Idle timeout                       Connection closed after 60 seconds without a

                                     pong







9.2 Client messages



  Message                                                      Meaning



   { "type": "subscribe",                                      Receive updates for one parcel

  "trackingNumber": "SDK7M2Q9XP4" }



   { "type": "unsubscribe",                                    Stop updates

  "trackingNumber": "SDK7M2Q9XP4" }



   { "type": "subscribe", "scope": "all"                       Receive all updates. DISPATCHER and

  }                                                            ADMIN only



   { "type": "ping" }                                          Keep alive







9.3 Server messages



   { "type": "subscribed", "trackingNumber": "SDK7M2Q9XP4" }





   {

       "type": "parcel.status.updated",

       "data": {

         "trackingNumber": "SDK7M2Q9XP4",

         "previousStatus": "IN_TRANSIT",

           "status": "OUT_FOR_DELIVERY",

           "updatedAt": "2026-11-02T15:40:00Z"

       }

   }





   { "type": "error", "code": "FORBIDDEN", "message": "Not allowed to subscribe

   to this scope." }





Update messages contain only public tracking fields. No names, addresses, phone

numbers or notes are sent.


<!-- PDF page 39 -->

9.4 Close codes



  Code           Meaning



  1000           Normal closure



  4401           Token invalid or expired. Refresh and reconnect



  4403           Not allowed



  4429           Too many subscriptions or messages









## 10. Outbound Merchant Webhooks

The Webhook Worker sends an HTTP POST to the merchant's configured URL whenever a

parcel linked to that merchant changes status. Each request is signed with HMAC SHA-

256.





10.1 Request

Headers:





  Header                                           Description



   Content-Type                                    application/json



   X-SwiftDrop-Event                               Event type, for example parcel.status_changed



   X-SwiftDrop-Event-Id                            Unique event ID. Use it to ignore duplicates



   X-SwiftDrop-Timestamp                           Unix time in seconds when the request was signed



   X-SwiftDrop-Signature                           sha256= followed by the lowercase hex HMAC SHA-256 of

                                                   the string <timestamp>.<raw body> using the

                                                   webhook signing secret





Body:


<!-- PDF page 40 -->

{

       "eventId": "6c8f8e56-5c6b-4d7c-9d9a-5f0f1f2b3c4d",

       "eventType": "parcel.status_changed",

       "schemaVersion": 1,

       "occurredAt": "2026-11-04T13:00:00Z",

       "data": {

         "trackingNumber": "SDN4T8W2LC6",

           "externalOrderId": "SZ-100245",

           "previousStatus": "PICKED_UP",

           "status": "IN_TRANSIT",

           "note": null,

           "failureReason": null

       }

   }

The internal RabbitMQ trigger is `parcel.status_changed.v1`. The public body remains camelCase with unversioned `eventType: parcel.status_changed` and additive `schemaVersion: 1` for API v1.1 compatibility. `eventId` is the stable outbox UUID. The webhook worker obtains `externalOrderId` through the implemented authorized internal parcel summary (`GET /internal/parcels/{parcelId}/summary`, section 13), loads the webhook and decrypted signing secret through the Merchant internal API, delivers the signed request, records the attempt, and schedules the 1m/5m/30m/2h/6h business retries.

10.2 Merchant responsibilities

       Verify the signature using the raw request body, compare signatures in constant

       time, and reject requests with a timestamp older than 5 minutes.

       Respond with any 2xx status within 10 seconds. Any other result counts as a failure.

       Treat eventId as an idempotency key because the same event can be delivered more

       than once.





10.3 Retry policy



  Attempt             Delay after the previous failure



  1                   Immediate



  2                   1 minute



  3                   5 minutes



  4                   30 minutes



  5                   2 hours



  6                   6 hours





After attempt 6 fails, the event moves to the dead letter queue and the attempt is marked

 deadLettered in the delivery log (endpoint 8.15). Every attempt is recorded.

These webhook business retries are separate from shared RabbitMQ consumer retries. The webhook worker must not re-enter the shared consumer retry loop for the same delivery attempt; doing so would multiply attempts. The current worker does not implement this delivery/retry behavior.


<!-- PDF page 41 -->

## 11. RabbitMQ Event Contracts

### 11.1 Topology

- Exchange: `swiftdrop.events` (durable topic exchange).
- Published routing keys: `parcel.created.v1`, `parcel.assigned.v1`, `parcel.status_changed.v1`.
- Target consumer bindings:

| Queue | Bindings | Consumer |
| --- | --- | --- |
| `notification.queue` | `parcel.created.v1`, `parcel.status_changed.v1` | Notification Worker |
| `webhook.queue` | `parcel.status_changed.v1` | Webhook Worker |
| `realtime.queue` | `parcel.assigned.v1`, `parcel.status_changed.v1` | Realtime Service |
| `merchant.sync.queue` | `parcel.status_changed.v1` | Merchant Integration |

Queue names are deployment details. The implemented durable queues are `notification.requested.v1` (legacy), `notification.parcel.created.v1`, `notification.parcel.status_changed.v1`, `webhook.parcel.status_changed.v1`, `realtime.parcel.status_changed.v1` and `merchant.sync.queue`; do not treat the target names in this table as evidence that they are running. The merchant service declares and consumes `merchant.sync.queue`; the event-flow integration test exercises the consumer against PostgreSQL and RabbitMQ. Do not rename or redeclare an existing durable queue with different arguments. A migration must introduce new queues or an explicit compatibility plan.

Failure topology is derived by the shared consumer: retry exchange `<exchange>.retry`, retry queue `<queue>.retry.v2`, dead-letter exchange `<exchange>.dlx`, and dead-letter queue `<queue>.dlq`. A retry is dead-lettered to its owning queue through the default exchange, so it cannot reach another worker bound to the same event. The legacy name `swiftdrop.dlx` is not the implemented exchange. Messages are persistent, and consumers deduplicate by `event_id`.

### 11.2 Envelope

Every internal event uses the canonical envelope from `packages/go-common/messaging.Envelope`:

```json
{
  "event_id": "6c8f8e56-5c6b-4d7c-9d9a-5f0f1f2b3c4d",
  "event_type": "parcel.status_changed.v1",
  "event_version": 1,
  "occurred_at": "2026-11-05T12:00:00Z",
  "producer": "parcel",
  "correlation_id": "req_01HF-example",
  "data": {
    "parcel_id": "6c8f8e56-5c6b-4d7c-9d9a-5f0f1f2b3c4d",
    "tracking_number": "SDN4T8W2LC6",
    "previous_status": "PICKED_UP",
    "status": "IN_TRANSIT"
  }
}
```

- `event_id` is the outbox UUID.
- `event_type` is versioned and must match `event_version`.
- `occurred_at` is RFC 3339.
- `producer` identifies the owning service.
- `correlation_id` is optional.
- `data` must be a JSON object and is defined by the event schema.

Do not use the flat camelCase API response/example envelope as the internal RabbitMQ envelope.

### 11.3 Event types

| Event type | Routing key | Published when | Data fields |
| --- | --- | --- | --- |
| `parcel.created.v1` | `parcel.created.v1` | A parcel is created. | `parcel_id`, `tracking_number`, `status` (`CREATED`), and exactly one of `customer_user_id` or `merchant_id` |
| `parcel.assigned.v1` | `parcel.assigned.v1` | Dispatch assigns or reassigns a parcel. | `parcel_id`, `tracking_number`, `courier_id`, `assigned_by`, optional `previous_courier_id` |
| `parcel.status_changed.v1` | `parcel.status_changed.v1` | A parcel status changes. | `parcel_id`, `tracking_number`, `previous_status`, `status`, optional `merchant_id`, optional `customer_user_id`, optional `actor_user_id`, optional `note`, `failure_reason` (required when `status` is `FAILED`, 5 to 255 characters) |

Domain events never carry phone numbers, email addresses, physical addresses, passwords, tokens, or signing secrets. Consumers that need contact details call the identity internal contact endpoint.

### 11.4 Outbox storage mapping

Database outbox storage labels remain unversioned to avoid a schema migration:

| Storage label | Stored routing key | Published event type | Published routing key |
| --- | --- | --- | --- |
| `parcel.created` | `parcel.created` | `parcel.created.v1` | `parcel.created.v1` |
| `parcel.assigned` | `parcel.assigned` | `parcel.assigned.v1` | `parcel.assigned.v1` |
| `parcel.status_changed` | `parcel.status.changed` | `parcel.status_changed.v1` | `parcel.status_changed.v1` |

The stored `routing_key` is legacy/internal. The relay maps the storage label and builds the canonical envelope; it must not copy the stored routing key blindly. The outbox writers and the relay are implemented; the M6 relay publishes committed versioned events with confirms and bounded retries.

Public merchant webhooks remain a separate projection with unversioned public `eventType: parcel.status_changed`; see section 10.

`notification.requested.v1` and `webhook.delivery.requested.v1` are legacy work-item contracts with no producers and are not part of the target parcel event flow. `notification.requested.v1` must not be exposed to merchants or used for new producers because its `recipient` field contains contact data. See [`contracts/events/README.md`](../contracts/events/README.md) for the current implementation status.

## 12. AI Delivery Assistant (Bonus)

Routed by Nginx from /api/v1/ai . The service calls an external language model with a

fixed set of tools. Every tool calls an existing SwiftDrop API with the caller's own

credentials, so normal authorization applies. The model has no database access and

cannot change data.





12.1 POST /ai/chat

Access: Public (rate limited). Tools that need a signed-in user return a safe message when

the caller is anonymous. Purpose: Answers parcel and pricing questions using results

from approved SwiftDrop API tools.



Request body:



   { "message": "How much to send 2 kg to Kandy?", "conversationId": null }





 message is required and limited to 500 characters. conversationId is optional and links

follow-up questions.


<!-- PDF page 43 -->

Response 200:



   {

       "data": {

         "conversationId": "cv81...",

       "reply": "Sending a 2 kg small parcel from Colombo to Kandy costs LKR

   550.00.",

       "toolCalls": [ { "name": "calculate_delivery_fee", "status": "OK" } ]

       },

       "meta": { "requestId": "req_40", "timestamp": "2026-11-05T09:00:00Z" }

   }





Errors: 422 (empty or too long message), 429, 503 AI_UNAVAILABLE (response includes a

fallback message asking the user to try again later).





12.2 Tools



  Tool                                             Calls                         Authorization



   calculate_delivery_fee                          POST /pricing/estimate        Public



   get_public_parcel_status                        GET                           Signed-in customer

                                                   /parcels/tracking/{            (own parcels) or

                                                   trackingNumber}                DISPATCHER/ADMIN



   get_my_parcel_history                           GET                           Signed-in owner only

                                                   /parcels/{parcelId}/history



   get_my_active_deliveries                        GET /parcels?status=...       Signed-in customer,

                                                                                 own parcels only





The service validates tool arguments, never accepts a user ID from the model, and

answers only from API results.





## 13. Internal Service APIs

These endpoints are used only between services on the internal Docker network. Nginx

does not route /internal . Each call must include X-Internal-Token and X-Request-

Id . They are not part of the public contract.


<!-- PDF page 44 -->

Method and path                                  Caller to      Purpose

                                                   owner



   POST /internal/pricing/quote                    Parcel to      Gets a fee quote.

                                                   Pricing        Same input and

                                                                  output as 3.1



   GET /internal/zones/{zoneId}                    Parcel to      Returns { "id",

                                                   Pricing        "code", "name",

                                                                  "isActive" }



   GET                                             Parcel to      Returns {

  /internal/couriers/{courierId}/eligibility       Identity       "isActive": true,

                                                                  "isAvailable":

                                                                  true, "role":

                                                                  "COURIER" } before

                                                                  assignment



   GET /internal/users/{userId}/contact            Notification   Returns {

                                                   to Identity    "fullName",

                                                                  "email", "phone" }



   GET                                             Notification   Returns {

  /internal/merchants/{merchantId}/contact         to Merchant    "merchantId",

                                                                  "ownerUserId",

                                                                  "businessName",

                                                                  "contactEmail",

                                                                  "status" } for

                                                                  the approved

                                                                  merchant contact



   POST /internal/parcels                          Merchant to    Creates a parcel for a

                                                   Parcel         merchant order



   GET /internal/parcels/{parcelId}/summary        Notification   Returns {

                                                   and            "parcelId",

                                                   Webhook to     "trackingNumber",

                                                   Parcel         "status",

                                                                  "customerUserId",

                                                                  "merchantId",

                                                                  "externalOrderId",

                                                                  "recipientName" }



   GET                                             Webhook        Returns { "url",

  /internal/merchants/{merchantId}/webhook         Worker to      "signingSecret",

                                                   Merchant       "eventTypes",

                                                                  "isActive",

                                                                  "merchantStatus" } .

                                                                  The worker sends only

                                                                  when merchantStatus is

                                                                  APPROVED; a suppressed

                                                                  delivery is recorded as

                                                                  a dead-lettered attempt.

                                                                  This is the only place

                                                                  the secret is returned



   POST                                            Webhook        Records one delivery

  /internal/merchants/{merchantId}/webhook/att     Worker to      attempt (fields as in

  empts                                            Merchant       8.15)





Merchant onboarding adds these internal endpoints. They are specified in
`contracts/openapi/identity-internal.yaml` and
`contracts/openapi/merchant-internal.yaml`, and described in
`docs/identity-internal-api.md` section 1a.

- `POST /internal/merchant-owners` (Merchant to Identity): creates or resumes an
  inactive MERCHANT account. 201 when created, 200 when resumed with the same
  password.
- `POST /internal/merchant-owners/{userId}/lifecycle` (Merchant to Identity):
  applies one APPROVE, REJECT or SUSPEND exactly once. 409 TRANSITION_ID_CONFLICT,
  STALE_TRANSITION or NOT_A_MERCHANT_OWNER.
- `DELETE /internal/merchant-owners/{userId}` (Merchant to Identity): takes back
  a failed registration. 409 OWNER_NOT_ELIGIBLE for any account that is not an
  inactive, unverified MERCHANT with no session and no transition.
- `GET /internal/users/{userId}/session-state` (Merchant to Identity): returns
  `role`, `isActive` and `sessionVersion`.
- `GET /internal/merchants/owners/{userId}/approval` (Identity to Merchant):
  returns `merchantId` and `status`, and for a REJECTED or SUSPENDED merchant the
  administrator's `reason`. Identity shows the reason to the owner after a
  correct password and does not log it.

Both internal Parcel endpoints are implemented and documented in
`contracts/openapi/parcel.yaml`. They require `X-Internal-Token`, fail closed when
it is not configured, and remain unreachable through the public gateway.
The trusted Merchant caller must supply the verified merchant owner's identity
user ID as `createdByUserId`. This completes the request contract: DB v1.1
requires `created_by_user_id` to identify the customer or merchant owner, and a
merchant ID is not a user ID. The M8 merchant order caller sends the verified
merchant owner's user ID; Parcel never derives it from the merchant ID.

 POST /internal/parcels request:


<!-- PDF page 45 -->

{

       "merchantId": "m1a2...",

       "createdByUserId": "<merchant-owner-user-uuid>",

       "externalOrderId": "SZ-100245",

     "idempotencyKey": "SZ-100245",

     "pickup": { "name": "ShopZone Warehouse", "phone": "+94112223344",

   "address": "88 Galle Road", "city": "Colombo", "zoneId": "7a1e..." },

     "recipient": { "name": "Amali Fernando", "phone": "+94712345678",

   "address": "45 Lake Drive", "city": "Kandy", "zoneId": "9b2f..." },

     "weightKg": 1.5,

       "sizeCategory": "SMALL",

       "description": "Phone case"

   }





Response 201: { "data": { "parcelId": "...", "trackingNumber": "SDN4T8W2LC6",

"status": "CREATED", "fee": { "currency": "LKR", "baseFee": 450.00,

"weightCharge": 100.00, "sizeSurcharge": 0.00, "total": 550.00 } } } .

The internal create route replays by `(merchantId, externalOrderId)`. A different
delivery under the same pair returns 409 `IDEMPOTENCY_CONFLICT`; an identical
delivery returns its original stored fee without repricing. The optional
`idempotencyKey` is retained as supplied, while external order uniqueness is the
database-enforced merchant replay boundary. Merchant approval and owner identity
verification remain the authenticated Merchant service's responsibility.







## 14. Health Endpoints

Every Go service exposes the following on its own port. They are used by Docker health

checks and are not routed publicly, except /healthz on the gateway.





  Method and                   Response                           Purpose

  path



   GET /health                 200 { "status": "ok" } when the    Liveness check

                               process is running



   GET /ready                  200 { "status": "ready",           Readiness check for PostgreSQL

                               "checks": { "database": "ok",      and RabbitMQ

                               "broker": "ok" } } , or 503 with

                               the failing check set to "down"



   GET                         Gateway only                       Confirms the public gateway

  /healthz                                                        process is responding









## 15. Endpoint Summary



  Section           Method              Path                                       Access



  2.1               POST                 /auth/register                            Public



  2.2               POST                 /auth/login                               Public


<!-- PDF page 46 -->

Section           Method              Path                             Access



  2.3               POST                 /auth/refresh                   Public (refresh

                                                                         token)



  2.4               POST                 /auth/logout                    Any user



  2.5               GET                  /auth/google/login              Public



  2.6               GET                  /auth/google/callback           Public



  2.7               GET                  /users/me                       Any user



  2.8               PATCH                /users/me                       Any user



  2.9               PUT                  /users/me/password              Any user



  2.10              GET                  /admin/users                    ADMIN



  2.11              POST                 /admin/users                    ADMIN



  2.12              GET                  /admin/users/{userId}           ADMIN



  2.13              PATCH                /admin/users/{userId}           ADMIN



  2.14              PATCH                /admin/users/{userId}/status    ADMIN



  2.15              GET                  /couriers                       DISPATCHER,

                                                                         ADMIN



  2.16              POST                 /auth/forgot-password           Public (rate

                                                                         limited)



  2.17              POST                 /auth/reset-password            Public (valid reset

                                                                         token)



  3.1               POST                 /pricing/estimate               Public



  3.2               GET                  /zones                          Public



  3.3               POST                 /admin/zones                    ADMIN



  3.4               PATCH                /admin/zones/{zoneId}           ADMIN



  3.5               GET                  /admin/pricing/rules            ADMIN



  3.6               POST                 /admin/pricing/rules            ADMIN



  3.7               PATCH                /admin/pricing/rules/{ruleId}   ADMIN



  3.8               GET                  /admin/pricing/bands            ADMIN



  3.9               POST                 /admin/pricing/bands            ADMIN


<!-- PDF page 47 -->

Section           Method              Path                                        Access



  3.10              PATCH                /admin/pricing/bands/{bandId}              ADMIN



  3.11              GET                  /admin/pricing/surcharges                  ADMIN



  3.12              PUT                  /admin/pricing/surcharges/{sizeCategory}   ADMIN



  3.13              GET                  /admin/zones                    ADMIN



  4.1               POST                 /parcels                                   CUSTOMER



  4.2               GET                  /parcels                                   CUSTOMER



  4.3               GET                  /parcels/{parcelId}                        CUSTOMER

                                                                                    (owner),

                                                                                    DISPATCHER,

                                                                                    ADMIN



  4.4               GET                  /parcels/{parcelId}/history                CUSTOMER

                                                                                    (owner),

                                                                                    DISPATCHER,

                                                                                    ADMIN



  4.5               GET                  /parcels/{parcelId}/receipt                CUSTOMER

                                                                                    (owner), ADMIN



  4.6               GET                  /tracking/{trackingNumber}                 Public



  5.1               GET                  /dispatch/unassigned                       DISPATCHER,

                                                                                    ADMIN



  5.2               PUT                  /dispatch/parcels/{parcelId}/assignment    DISPATCHER,

                                                                                    ADMIN



  5.3               GET                  /dispatch/workload                         DISPATCHER,

                                                                                    ADMIN



  5.4               GET                  /admin/parcels                             DISPATCHER,

                                                                                    ADMIN



  6.1               GET                  /courier/deliveries                        COURIER



  6.2               GET                  /courier/deliveries/{parcelId}             COURIER

                                                                                    (assigned)



  6.3               POST                 /courier/deliveries/{parcelId}/status      COURIER

                                                                                    (assigned)



  6.4               POST                 /courier/deliveries/{parcelId}/proof       COURIER

                                                                                    (assigned)


<!-- PDF page 48 -->

Section           Method              Path                                     Access



  7.1               GET                  /reports/daily                          DISPATCHER,

                                                                                 ADMIN



  7.2               GET                  /reports/outcomes                       DISPATCHER,

                                                                                 ADMIN



  8.1               POST                 /merchants/register                     Public



  8.2, 8.3          GET,                 /merchants/me                           MERCHANT

                    PATCH



  8.4               GET                  /admin/merchants                        ADMIN



  8.5               GET                  /admin/merchants/{merchantId}           ADMIN



  8.6               POST                 /admin/merchants/{merchantId}/approve   ADMIN



  8.7               POST                 /admin/merchants/{merchantId}/reject    ADMIN



  8.8               POST                 /admin/merchants/{merchantId}/suspend   ADMIN



  8.9, 8.10         GET,                 /merchants/me/keys                      MERCHANT

                    POST                                                         (POST: approved

                                                                                 only)



  8.11              DELETE               /merchants/me/keys/{keyId}              MERCHANT



  8.12,             GET,                 /merchants/me/webhook                   MERCHANT (PUT:

  8.13              PUT                                                          approved only)



  8.14              POST                 /merchants/me/webhook/test              MERCHANT



  8.15              GET                  /merchants/me/webhook/deliveries        MERCHANT



  8.16              POST                 /merchant/orders                        Merchant API key



  8.17              GET                  /merchant/orders                        MERCHANT (JWT)

                                                                                 or API key



  8.18              GET                  /merchant/orders/{orderId}              MERCHANT (JWT)

                                                                                 or API key



  12.1              POST                 /ai/chat                                Public (rate

                                                                                 limited)



  9                 WS                   /ws                                     Public or

                                                                                 authenticated
