package releasegate

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/hurtener/chartworks/internal/store/postgres"
)

func command(ctx context.Context, dir, name string, args ...string) (string, error) {
	c := exec.CommandContext(ctx, name, args...)
	c.Dir = dir
	out, err := c.Output()
	if err != nil {
		return "", fmt.Errorf("%w: release command %s failed", ErrEvidence, name)
	}
	return strings.TrimSpace(string(out)), nil
}

type schemaManifest struct {
	Commit         string `json:"commit"`
	MigrationCount int    `json:"migration_count"`
	SHA256         string `json:"sha256"`
}

func verifySchemaOutput(output, head, digest string) error {
	var manifest schemaManifest
	if err := decodeExact([]byte(output), &manifest); err != nil || manifest.Commit != head || manifest.SHA256 != digest || manifest.MigrationCount < 1 {
		return fmt.Errorf("%w: release schema differs from source", ErrEvidence)
	}
	return nil
}

// VerifyArtifacts compares the release binary's embedded migration manifest to
// this source build and the candidate manifest. It also pins a clean Git tree,
// container image, active docs, generated API response and human review.
func VerifyArtifacts(root, dir string, b Bundle) error {
	if b.Artifacts.SourceTree == "" || !hex40.MatchString(b.Artifacts.SourceTree) || b.Artifacts.ImageRef == "" || !strings.HasPrefix(b.Artifacts.ImageDigest, "sha256:") || !hex64.MatchString(strings.TrimPrefix(b.Artifacts.ImageDigest, "sha256:")) {
		return fmt.Errorf("%w: release artifact identity missing", ErrEvidence)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	head, err := command(ctx, root, "git", "rev-parse", "HEAD")
	if err != nil || head != b.Head {
		return fmt.Errorf("%w: wrong source head", ErrEvidence)
	}
	tree, err := command(ctx, root, "git", "rev-parse", "HEAD^{tree}")
	if err != nil || tree != b.Artifacts.SourceTree {
		return fmt.Errorf("%w: wrong source tree", ErrEvidence)
	}
	status, err := command(ctx, root, "git", "status", "--porcelain", "--untracked-files=all")
	if err != nil || status != "" {
		return fmt.Errorf("%w: release source tree is dirty", ErrEvidence)
	}
	if err := VerifySourceFiles(root, b.Artifacts.Files); err != nil {
		return err
	}
	if err := VerifyAPISchema(root, dir, b.Artifacts.APISchema); err != nil {
		return err
	}
	if err := VerifyReview(dir, b); err != nil {
		return err
	}
	schema, err := postgres.SchemaDigest()
	if err != nil || schema != b.Artifacts.MigrationsSHA {
		return fmt.Errorf("%w: migration digest differs from source", ErrEvidence)
	}
	binary, err := VerifyFile(dir, b.Artifacts.Binary, 512<<20)
	if err != nil {
		return err
	}
	version, err := command(ctx, dir, binary, "version")
	if err != nil || !strings.Contains(version, "("+b.Head+";") {
		return fmt.Errorf("%w: release binary version differs from source", ErrEvidence)
	}
	out, err := command(ctx, dir, binary, "schema-manifest")
	if err != nil {
		return err
	}
	if err := verifySchemaOutput(out, b.Head, schema); err != nil {
		return err
	}
	image, err := command(ctx, root, "docker", "image", "inspect", "--format", "{{.Id}}", b.Artifacts.ImageRef)
	if err != nil || image != b.Artifacts.ImageDigest {
		return fmt.Errorf("%w: reference image digest mismatch", ErrEvidence)
	}
	imageVersion, err := command(ctx, root, "docker", "run", "--rm", "--network", "none", "--read-only", b.Artifacts.ImageRef, "version")
	if err != nil || !strings.Contains(imageVersion, "("+b.Head+";") {
		return fmt.Errorf("%w: reference image binary version mismatch", ErrEvidence)
	}
	imageSchema, err := command(ctx, root, "docker", "run", "--rm", "--network", "none", "--read-only", b.Artifacts.ImageRef, "schema-manifest")
	if err != nil {
		return err
	}
	if err := verifySchemaOutput(imageSchema, b.Head, schema); err != nil {
		return err
	}
	return nil
}
