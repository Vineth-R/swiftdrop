//go:build integration

package repository

import (
	"errors"
	"testing"
)

func TestMerchantRepositoryLifecycleAndList(t *testing.T) {
	ctx, _, repo := newOrderTestPool(t)
	merchantID, ownerID := createOrderMerchant(t, ctx, repo)

	merchant, err := repo.MerchantByID(ctx, merchantID)
	if err != nil || merchant.OwnerUserID != ownerID || merchant.Status != MerchantStatusPending {
		t.Fatalf("MerchantByID() = %+v, %v", merchant, err)
	}
	if _, err := repo.MerchantByOwnerUserID(ctx, ownerID); err != nil {
		t.Fatalf("MerchantByOwnerUserID() error = %v", err)
	}
	updated, err := repo.UpdateProfile(ctx, merchantID, ProfileUpdate{
		ContactEmail: "new@example.test", ContactPhone: "+94770000000",
		PickupAddress: "99 New Road", PickupZoneID: orderUUID(),
	})
	if err != nil || updated.ContactEmail != "new@example.test" || updated.PickupAddress != "99 New Road" {
		t.Fatalf("UpdateProfile() = %+v, %v", updated, err)
	}

	transitionID := orderUUID()
	if _, transition, err := repo.BeginApprovalTransition(ctx, merchantID, orderUUID(), transitionID); err != nil || transition.SyncStatus != "PENDING" {
		t.Fatalf("BeginApprovalTransition() = %+v, %v", transition, err)
	}
	approved, err := repo.ApplyApprovalTransition(ctx, merchantID, orderUUID(), transitionID)
	if err != nil || approved.Status != MerchantStatusApproved || approved.ApprovedAt == nil {
		t.Fatalf("ApplyApprovalTransition() = %+v, %v", approved, err)
	}
	if latest, err := repo.LatestTransition(ctx, merchantID, MerchantStatusApproved); err != nil || latest.SyncStatus != "APPLIED" {
		t.Fatalf("LatestTransition() = %+v, %v", latest, err)
	}
	pending, err := repo.PendingIdentitySyncs(ctx, []string{merchantID})
	if err != nil || pending[merchantID] {
		t.Fatalf("PendingIdentitySyncs(after approval) = %+v, %v", pending, err)
	}

	suspendID := orderUUID()
	reason := "chargebacks"
	if _, transition, err := repo.BeginSuspensionTransition(ctx, merchantID, orderUUID(), suspendID, &reason); err != nil || transition.SyncStatus != "PENDING" {
		t.Fatalf("BeginSuspensionTransition() = %+v, %v", transition, err)
	}
	pending, err = repo.PendingIdentitySyncs(ctx, []string{merchantID})
	if err != nil || !pending[merchantID] {
		t.Fatalf("PendingIdentitySyncs(after suspension) = %+v, %v", pending, err)
	}
	if err := repo.MarkTransitionResult(ctx, suspendID, true, ""); err != nil {
		t.Fatalf("MarkTransitionResult() error = %v", err)
	}
	pending, err = repo.PendingIdentitySyncs(ctx, []string{merchantID})
	if err != nil || pending[merchantID] {
		t.Fatalf("PendingIdentitySyncs(after applied) = %+v, %v", pending, err)
	}
	if err := repo.MarkTransitionResult(ctx, suspendID, false, "identity down"); err != nil {
		t.Fatalf("MarkTransitionResult(failed) error = %v", err)
	}

	merchants, total, err := repo.ListMerchants(ctx, MerchantFilter{Sort: "businessName", Desc: false, Limit: 10})
	if err != nil || total < 1 || len(merchants) == 0 {
		t.Fatalf("ListMerchants() = %d/%d, %v", len(merchants), total, err)
	}
	if _, _, err := repo.ListMerchants(ctx, MerchantFilter{Status: string(MerchantStatusApproved), Limit: 10}); err != nil {
		t.Fatalf("ListMerchants(status) error = %v", err)
	}
	if _, err := repo.MerchantByID(ctx, orderUUID()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("MerchantByID(missing) error = %v", err)
	}
}

func TestAPIClientRepository(t *testing.T) {
	ctx, _, repo := newOrderTestPool(t)
	merchantID, _ := createOrderMerchant(t, ctx, repo)

	for i := 0; i < 5; i++ {
		params := CreateAPIClientParams{
			ID: orderUUID(), MerchantID: merchantID, Name: "key",
			KeyPrefix: "sdk_live_abcd", LookupID: orderHex(8), KeyHash: orderHex(32),
		}
		if err := repo.CreateAPIClient(ctx, params); err != nil {
			t.Fatalf("CreateAPIClient(%d) error = %v", i, err)
		}
	}
	if err := repo.CreateAPIClient(ctx, CreateAPIClientParams{
		ID: orderUUID(), MerchantID: merchantID, Name: "sixth",
		KeyPrefix: "sdk_live_abcd", LookupID: orderHex(8), KeyHash: orderHex(32),
	}); !errors.Is(err, ErrKeyLimit) {
		t.Fatalf("sixth CreateAPIClient() error = %v, want ErrKeyLimit", err)
	}

	keys, err := repo.APIClientsByMerchant(ctx, merchantID)
	if err != nil || len(keys) != 5 {
		t.Fatalf("APIClientsByMerchant() = %d, %v", len(keys), err)
	}
	lookup := orderHex(8)
	hash := orderHex(32)
	params := CreateAPIClientParams{ID: orderUUID(), MerchantID: merchantID, Name: "lookup", KeyPrefix: "sdk_live_ab", LookupID: lookup, KeyHash: hash}
	_ = repo.CreateAPIClient(ctx, params) // sixth may fail; the row must already exist for lookup
	client, stored, err := repo.APIClientByLookup(ctx, lookup)
	if err != nil {
		// The limit is five, so revoke one first and retry.
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("APIClientByLookup() error = %v", err)
		}
		if err := repo.RevokeAPIClient(ctx, merchantID, keys[0].ID); err != nil {
			t.Fatalf("RevokeAPIClient() error = %v", err)
		}
		if err := repo.CreateAPIClient(ctx, params); err != nil {
			t.Fatalf("CreateAPIClient(after revoke) error = %v", err)
		}
		client, stored, err = repo.APIClientByLookup(ctx, lookup)
	}
	if err != nil || stored != hash || client.MerchantID != merchantID {
		t.Fatalf("APIClientByLookup() = %+v %q, %v", client, stored, err)
	}
	if err := repo.TouchAPIClient(ctx, client.ID); err != nil {
		t.Fatalf("TouchAPIClient() error = %v", err)
	}
	if err := repo.RevokeAPIClient(ctx, merchantID, client.ID); err != nil {
		t.Fatalf("RevokeAPIClient() error = %v", err)
	}
	if err := repo.RevokeAPIClient(ctx, merchantID, client.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second RevokeAPIClient() error = %v, want ErrNotFound", err)
	}
}

func TestRegistrationRepository(t *testing.T) {
	ctx, _, repo := newOrderTestPool(t)
	attemptID, key := orderUUID(), "registration-key"
	attempt, err := repo.EnsureRegistrationAttempt(ctx, attemptID, key, "new@example.test")
	if err != nil || attempt.ID != attemptID || attempt.Status != "PENDING" {
		t.Fatalf("EnsureRegistrationAttempt() = %+v, %v", attempt, err)
	}
	same, err := repo.EnsureRegistrationAttempt(ctx, orderUUID(), key, "new@example.test")
	if err != nil || same.ID != attemptID {
		t.Fatalf("idempotent EnsureRegistrationAttempt() = %+v, %v", same, err)
	}
	ownerID := orderUUID()
	if err := repo.SetRegistrationAttemptUser(ctx, attemptID, ownerID); err != nil {
		t.Fatalf("SetRegistrationAttemptUser() error = %v", err)
	}
	params := CreateMerchantParams{
		ID: orderUUID(), OwnerUserID: ownerID, BusinessName: "Registration Shop",
		BusinessRegNo: "REG-2-" + ownerID[:8], ContactEmail: "reg@example.test",
		ContactPhone: "+94771234567", PickupAddress: "1 Road", PickupZoneID: orderUUID(),
	}
	merchant, err := repo.CreatePendingMerchant(ctx, attemptID, params)
	if err != nil || merchant.Status != MerchantStatusPending {
		t.Fatalf("CreatePendingMerchant() = %+v, %v", merchant, err)
	}
	// A second attempt with the same owner is a duplicate application.
	attempt2, err := repo.EnsureRegistrationAttempt(ctx, orderUUID(), "registration-key-2", "second@example.test")
	if err != nil {
		t.Fatalf("EnsureRegistrationAttempt(second) error = %v", err)
	}
	duplicate := params
	duplicate.ID = orderUUID()
	duplicate.BusinessRegNo = "REG 2 " + orderHex(6)
	if _, err := repo.CreatePendingMerchant(ctx, attempt2.ID, duplicate); !errors.Is(err, ErrDuplicateApplication) {
		t.Fatalf("duplicate owner error = %v, want ErrDuplicateApplication", err)
	}
	if err := repo.FailRegistrationAttempt(ctx, attempt2.ID, "identity down"); err != nil {
		t.Fatalf("FailRegistrationAttempt() error = %v", err)
	}
	if err := repo.CompleteRegistrationAttempt(ctx, attempt2.ID, merchant.ID); err != nil {
		t.Fatalf("CompleteRegistrationAttempt() error = %v", err)
	}
}

func TestWebhookRepositoryAndDeliveryAttempts(t *testing.T) {
	ctx, _, repo := newOrderTestPool(t)
	merchantID, _ := createOrderMerchant(t, ctx, repo)

	if _, err := repo.WebhookByMerchant(ctx, merchantID); !errors.Is(err, ErrWebhookNotConfigured) {
		t.Fatalf("WebhookByMerchant(before) error = %v", err)
	}
	if _, err := repo.SaveWebhook(ctx, SaveWebhookParams{ID: orderUUID(), MerchantID: merchantID, URL: "https://shop.test/hook"}); err == nil {
		t.Fatal("SaveWebhook() accepted a missing secret")
	}
	created, err := repo.SaveWebhook(ctx, SaveWebhookParams{
		ID: orderUUID(), MerchantID: merchantID, URL: "https://shop.test/hook",
		EventTypes: []string{"parcel.status_changed"}, IsActive: true, CandidateSecretEnc: []byte("enc-1"),
	})
	if err != nil || !created.SecretStored || created.Webhook.URL != "https://shop.test/hook" {
		t.Fatalf("SaveWebhook(create) = %+v, %v", created, err)
	}
	kept, err := repo.SaveWebhook(ctx, SaveWebhookParams{
		MerchantID: merchantID, URL: "https://shop.test/new", EventTypes: []string{"parcel.status_changed"},
		IsActive: true, CandidateSecretEnc: []byte("enc-2"),
	})
	if err != nil || kept.SecretStored || kept.Webhook.URL != "https://shop.test/new" {
		t.Fatalf("SaveWebhook(keep) = %+v, %v", kept, err)
	}
	if string(kept.Webhook.SigningSecretEnc) != "enc-1" {
		t.Fatalf("SaveWebhook(keep) changed the stored secret: %q", kept.Webhook.SigningSecretEnc)
	}
	rotated, err := repo.SaveWebhook(ctx, SaveWebhookParams{
		MerchantID: merchantID, URL: "https://shop.test/new", EventTypes: []string{"parcel.status_changed"},
		IsActive: true, RotateSecret: true, CandidateSecretEnc: []byte("enc-3"),
	})
	if err != nil || !rotated.SecretStored || string(rotated.Webhook.SigningSecretEnc) != "enc-3" {
		t.Fatalf("SaveWebhook(rotate) = %+v, %v", rotated, err)
	}

	attemptID := orderUUID()
	attempt := RecordAttemptParams{
		ID: attemptID, MerchantID: merchantID, EventID: orderUUID(), ParcelID: orderUUID(),
		TrackingNumber: "SDN00000001", AttemptNumber: 1, Success: true,
	}
	recorded, err := repo.RecordDeliveryAttempt(ctx, attempt)
	if err != nil || !recorded {
		t.Fatalf("RecordDeliveryAttempt() = %v, %v", recorded, err)
	}
	recorded, err = repo.RecordDeliveryAttempt(ctx, attempt)
	if err != nil || recorded {
		t.Fatalf("duplicate RecordDeliveryAttempt() = %v, %v", recorded, err)
	}
	success := true
	attempts, total, err := repo.ListDeliveryAttempts(ctx, merchantID, DeliveryFilter{Success: &success, TrackingNumber: "SDN00000001", Limit: 10})
	if err != nil || total != 1 || len(attempts) != 1 || !attempts[0].Success {
		t.Fatalf("ListDeliveryAttempts() = %+v total=%d, %v", attempts, total, err)
	}
	if _, err := repo.RecordDeliveryAttempt(ctx, RecordAttemptParams{
		ID: orderUUID(), MerchantID: orderUUID(), EventID: orderUUID(), ParcelID: orderUUID(),
		TrackingNumber: "SDN00000002", AttemptNumber: 1,
	}); !errors.Is(err, ErrWebhookNotConfigured) {
		t.Fatalf("RecordDeliveryAttempt(no webhook) error = %v", err)
	}
}

func TestOrderRepositoryRejectsUnknownMerchant(t *testing.T) {
	ctx, _, repo := newOrderTestPool(t)
	if _, err := repo.CreateOrder(ctx, orderParams(orderUUID(), "SZ-NO-MERCHANT")); err == nil {
		t.Fatal("CreateOrder() accepted an unknown merchant")
	}
}
