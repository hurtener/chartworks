package chartworks

import (
	"context"
	"errors"
	"testing"
)

// Malformed literal coordinates must fail before any client, token provider or transport can be used.
func TestBlockSDKRejectsCoordinatesBeforeCredentials(t *testing.T) {
	ctx := context.Background()
	var c *Client
	calls := []struct {
		name string
		call func(string) error
	}{
		{"ParameterizeBlock", func(id string) error { _, err := c.ParameterizeBlock(ctx, id, BlockParameterizeRequest{}); return err }},
		{"RecheckBlockImpact", func(id string) error { _, err := c.RecheckBlockImpact(ctx, id, BlockImpactRequest{}); return err }},
		{"ApplyBlockImpact", func(id string) error { _, err := c.ApplyBlockImpact(ctx, id, BlockApplyImpactRequest{}); return err }},
		{"ReadBlock", func(id string) error { _, err := c.ReadBlock(ctx, id, BlockReference{}); return err }},
		{"ReadBlockSQL", func(id string) error { _, err := c.ReadBlockSQL(ctx, id, BlockReference{}); return err }},
		{"BlockHistory", func(id string) error { _, err := c.BlockHistory(ctx, id); return err }},
		{"EditBlock", func(id string) error { _, err := c.EditBlock(ctx, id, BlockEditRequest{}); return err }},
		{"ValidateBlock", func(id string) error { _, err := c.ValidateBlock(ctx, id, BlockValidateRequest{}); return err }},
		{"PreviewBlock", func(id string) error { _, err := c.PreviewBlock(ctx, id, BlockPreviewRequest{}); return err }},
		{"PublishBlock", func(id string) error { _, err := c.PublishBlock(ctx, id, BlockPublishRequest{}); return err }},
		{"CertifyBlock", func(id string) error { _, err := c.CertifyBlock(ctx, id, BlockCertifyRequest{}); return err }},
		{"WithdrawBlockCertification", func(id string) error {
			_, err := c.WithdrawBlockCertification(ctx, id, BlockWithdrawRequest{})
			return err
		}},
		{"RejectBlock", func(id string) error { _, err := c.RejectBlock(ctx, id, BlockTransitionRequest{}); return err }},
		{"RestoreBlock", func(id string) error { _, err := c.RestoreBlock(ctx, id, BlockRestoreRequest{}); return err }},
		{"ArchiveBlock", func(id string) error { _, err := c.ArchiveBlock(ctx, id, BlockTransitionRequest{}); return err }},
		{"ResolveBlockParameters", func(id string) error { _, err := c.ResolveBlockParameters(ctx, id, BlockResolveRequest{}); return err }},
	}
	for _, tc := range calls {
		t.Run(tc.name, func(t *testing.T) {
			for _, id := range []string{"", "../other", "a/b", "%2e%2e", "space id"} {
				if err := tc.call(id); !errors.Is(err, ErrBlockRequest) {
					t.Fatal(id, err)
				}
			}
		})
	}
	for _, ref := range []BlockReference{{Revision: -1}, {Revision: 257}, {Revision: 1, Draft: true}} {
		if _, err := c.ReadBlock(ctx, "block", ref); !errors.Is(err, ErrBlockRequest) {
			t.Fatal(ref, err)
		}
		if _, err := c.ReadBlockSQL(ctx, "block", ref); !errors.Is(err, ErrBlockRequest) {
			t.Fatal(ref, err)
		}
	}
	for _, in := range []BlockListRequest{{Limit: -1}, {Limit: 101}, {After: "../other"}} {
		if _, err := c.ListBlocks(ctx, in); !errors.Is(err, ErrBlockRequest) {
			t.Fatal(in, err)
		}
	}
}
