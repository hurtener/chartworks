package releasegate

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
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

// VerifyArtifacts compares the release binary's embedded migration manifest to
// this source build and the candidate manifest. It also pins a clean Git tree,
// container image, active docs, generated API response and human review.
func VerifyArtifacts(root, dir string, b Bundle) error {
	if b.Artifacts.SourceTree == "" || !hex40.MatchString(b.Artifacts.SourceTree) || b.Artifacts.ImageRef == "" || !strings.HasPrefix(b.Artifacts.ImageDigest, "sha256:") || !hex64.MatchString(strings.TrimPrefix(b.Artifacts.ImageDigest, "sha256:")) {
		return fmt.Errorf("%w: release artifact identity missing", ErrEvidence)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
	if err := VerifyAPISchema(dir, b.Artifacts.APISchema); err != nil {
		return err
	}
	if err := VerifyReview(dir, b); err != nil {
		return err
	}
	schema, err := postgres.SchemaDigest()
	if err != nil || schema != b.Artifacts.MigrationsSHA {
		return fmt.Errorf("%w: migration digest differs from source", ErrEvidence)
	}
	if _, err := ReadFile(dir, b.Artifacts.Binary, 512<<20); err != nil {
		return err
	}
	binary := filepath.Join(dir, b.Artifacts.Binary.Path)
	version, err := command(ctx, dir, binary, "version")
	if err != nil || !strings.Contains(version, "("+b.Head+";") {
		return fmt.Errorf("%w: release binary version differs from source", ErrEvidence)
	}
	out, err := command(ctx, dir, binary, "schema-manifest")
	if err != nil {
		return err
	}
	var manifest struct {
		Commit         string `json:"commit"`
		MigrationCount int    `json:"migration_count"`
		SHA256         string `json:"sha256"`
	}
	if err := decodeExact([]byte(out), &manifest); err != nil || manifest.Commit != b.Head || manifest.SHA256 != schema || manifest.MigrationCount < 1 {
		return fmt.Errorf("%w: release binary schema differs from source", ErrEvidence)
	}
	image, err := command(ctx, root, "docker", "image", "inspect", "--format", "{{.Id}}", b.Artifacts.ImageRef)
	if err != nil || image != b.Artifacts.ImageDigest {
		return fmt.Errorf("%w: reference image digest mismatch", ErrEvidence)
	}
	return nil
}
