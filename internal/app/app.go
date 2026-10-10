// Package app wires application dependencies and runtime components.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"
	"unicode/utf8"

	"github.com/tekmora/discord-bot/internal/answer"
	"github.com/tekmora/discord-bot/internal/config"
	"github.com/tekmora/discord-bot/internal/corpus"
	"github.com/tekmora/discord-bot/internal/model"
	"github.com/tekmora/discord-bot/internal/retrieval"
)

// Assembly holds the initialized components and metadata of the application.
type Assembly struct {
	Pipeline *answer.Pipeline
	Chunks   int
	Sources  []corpus.ManifestSource
	HasModel bool
}

// Build constructs and validates the application runtime from cfg.
//
// Build opens the corpus directory with os.OpenRoot so no symbolic link can leave it.
// The returned cleanup function closes the opened root; double-close is safe and will not panic.
func Build(ctx context.Context, cfg config.Config, logger *slog.Logger) (*Assembly, func() error, error) {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}

	root, err := os.OpenRoot(cfg.CorpusDir) // #nosec G304 -- CorpusDir is operator configuration.
	if err != nil {
		return nil, nil, fmt.Errorf("open corpus root: %w", err)
	}

	var closed atomic.Bool
	cleanup := func() error {
		if closed.Swap(true) {
			return nil
		}
		if closeErr := root.Close(); closeErr != nil {
			return fmt.Errorf("close corpus root: %w", closeErr)
		}
		return nil
	}

	manifest, chunks, err := loadCorpusFromRoot(ctx, root)
	if err != nil {
		return nil, nil, closeOnError(cleanup, err)
	}

	index, err := retrieval.Build(chunks)
	if err != nil {
		return nil, nil, closeOnError(cleanup, fmt.Errorf("build retrieval index: %w", err))
	}

	pipeline, err := buildPipeline(cfg, index, logger)
	if err != nil {
		return nil, nil, closeOnError(cleanup, err)
	}

	logAssemblyReady(logger, cfg, manifest.Sources, len(chunks))

	assembly := &Assembly{
		Pipeline: pipeline,
		Chunks:   len(chunks),
		Sources:  manifest.Sources,
		HasModel: cfg.HasModel(),
	}
	return assembly, cleanup, nil
}

func loadCorpusFromRoot(ctx context.Context, root *os.Root) (corpus.Manifest, []corpus.Chunk, error) {
	fsys := root.FS()
	manifest, err := corpus.LoadManifest(fsys)
	if err != nil {
		return corpus.Manifest{}, nil, fmt.Errorf("load manifest: %w", err)
	}

	chunks, err := corpus.LoadAll(ctx, fsys, manifest, corpus.DefaultLimits())
	if err != nil {
		return corpus.Manifest{}, nil, fmt.Errorf("load corpus: %w", err)
	}
	return manifest, chunks, nil
}

func buildPipeline(cfg config.Config, index *retrieval.Index, logger *slog.Logger) (*answer.Pipeline, error) {
	var generator answer.Generator
	if cfg.HasModel() {
		mCfg := model.Config{
			BaseURL: cfg.ModelBaseURL,
			Alias:   cfg.ModelAlias,
			APIKey:  cfg.ModelKey,
		}
		mClient, err := model.New(mCfg, nil)
		if err != nil {
			return nil, fmt.Errorf("create model client: %w", err)
		}
		generator = mClient
	}

	ansCfg := answer.Config{
		Retriever: index,
		Generator: generator,
		Logger:    logger,
	}
	pipeline, err := answer.New(ansCfg)
	if err != nil {
		return nil, fmt.Errorf("create answer pipeline: %w", err)
	}
	return pipeline, nil
}

// logAssemblyReady writes the one line that says what the process will answer from. The sources
// form a list of "repository@revision" entries: one key per source would repeat the key.
func logAssemblyReady(logger *slog.Logger, cfg config.Config, sources []corpus.ManifestSource, chunkCount int) {
	inventory := make([]string, len(sources))
	for i, s := range sources {
		rev := s.Revision
		if utf8.RuneCountInString(rev) > 12 {
			rev = string([]rune(rev)[:12])
		}
		inventory[i] = s.Repository + "@" + rev
	}

	attrs := []any{
		slog.Int("sources", len(sources)),
		slog.Int("chunks", chunkCount),
		slog.Bool("has_model", cfg.HasModel()),
		slog.Any("inventory", inventory),
	}
	if cfg.HasModel() {
		attrs = append(attrs, slog.String("model_alias", cfg.ModelAlias))
	}
	logger.Info("application assembly ready", attrs...)
}

func closeOnError(cleanup func() error, err error) error {
	if closeErr := cleanup(); closeErr != nil {
		return errors.Join(err, closeErr)
	}
	return err
}
