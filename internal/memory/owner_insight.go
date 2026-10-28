package memory

import (
	"context"
	"encoding/json"
	"github.com/humzakhan/capsule-memory/internal/frame"
	"github.com/humzakhan/capsule-memory/internal/records"
	"github.com/humzakhan/capsule-memory/internal/strictjson"
)

const ownerInsightDomain = "capsule-memory/independent-insight/v1"

func (s *Service) VerifyOwnerInsight(r Revision) error { return s.verifyOwnerInsight(r) }
func (s *Service) verifyOwnerInsight(r Revision) error {
	if r.Version != 2 || r.Kind != records.KindInsight || r.Operation == "delete" {
		return nil
	}
	var in records.Insight
	// Rebuild calls this directly: shape checks cannot depend on prior decryption.
	if strictjson.Decode(r.Body, frame.MaxDecoded, &in) != nil || in.Validate() != nil {
		return ErrInvalid
	}
	if in.Reviewer != "" && in.Reviewer != r.Author {
		return ErrInvalid
	}
	if in.OwnerProof == nil {
		return nil
	}
	var proof records.OwnerAssertionBody
	if s.VerifyOwner(ownerInsightDomain, Signed{Payload: in.OwnerProof.Payload, Signature: in.OwnerProof.Signature}, &proof) != nil || proof.Validate() != nil {
		return ErrInvalid
	}
	if proof.Vault != r.Vault || proof.Scope != r.Scope || proof.Record != r.Record || proof.Revision != r.ID || proof.OperationID != r.OperationID || proof.Operation != r.Operation || proof.Author != r.Author || !equalStrings(proof.Parents, r.Parents) || !equalStrings(proof.Restores, r.Restores) {
		return ErrInvalid
	}
	in.OwnerProof = nil
	raw, e := json.Marshal(in)
	if e != nil || digest(raw) != proof.ContentSHA256 {
		return ErrInvalid
	}
	return nil
}

func (s *Service) PrepareOwnerInsight(ctx context.Context, in PutInput) (Prepared, error) {
	if _, e := s.ownerKey(); e != nil {
		return Prepared{}, e
	}
	body, ok := in.Body.(records.Insight)
	if !ok || in.Kind != records.KindInsight || body.OwnerProof != nil || body.Generator != nil || body.Basis != "explicit" || (body.Review != "accepted" && body.Review != "edited") || len(body.Evidence) != 0 || len(body.Contradictions) != 0 || len(body.Influence) != 0 || body.AutoAccepted || (in.Operation != "" && in.Operation != "put" && in.Operation != "restore") {
		return Prepared{}, ErrInvalid
	}
	return s.prepare(ctx, in, func(r *Revision) error {
		if body.Reviewer != r.Author {
			return ErrInvalid
		}
		raw, e := json.Marshal(body)
		if e != nil {
			return e
		}
		proof := records.OwnerAssertionBody{Version: 1, Vault: r.Vault, Scope: r.Scope, Record: r.Record, Revision: r.ID, OperationID: r.OperationID, Operation: r.Operation, Parents: append([]string{}, r.Parents...), Restores: append([]string{}, r.Restores...), Author: r.Author, ContentSHA256: digest(raw)}
		signed, e := s.OwnerSign(ownerInsightDomain, proof)
		if e != nil {
			return e
		}
		body.OwnerProof = &records.OwnerProof{Payload: signed.Payload, Signature: signed.Signature}
		r.Body, e = json.Marshal(body)
		return e
	})
}
