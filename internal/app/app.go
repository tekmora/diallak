// Package app wires application dependencies and runtime components.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/tekmora/diallak/internal/answer"
	"github.com/tekmora/diallak/internal/config"
	"github.com/tekmora/diallak/internal/corpus"
	"github.com/tekmora/diallak/internal/embed"
	"github.com/tekmora/diallak/internal/model"
	"github.com/tekmora/diallak/internal/retrieval"
)

// HybridStartTimeout bounds the startup indexing phase for computing chunk embeddings.
// Source: chosen, not measured; gives generous headroom for embedding a few hundred chunks at start.
const HybridStartTimeout = 2 * time.Minute

// Assembly holds the initialized components and metadata of the application.
type Assembly struct {
	Pipeline      *answer.Pipeline
	Chunks        int
	Sources       []corpus.ManifestSource
	HasModel      bool
	Model         *model.Gate
	RenderOptions answer.RenderOptions
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

	pipeline, gate, err := buildPipeline(ctx, cfg, index, chunks, logger)
	if err != nil {
		return nil, nil, closeOnError(cleanup, err)
	}

	logAssemblyReady(logger, cfg, manifest.Sources, len(chunks))

	assembly := &Assembly{
		Pipeline: pipeline,
		Chunks:   len(chunks),
		Sources:  manifest.Sources,
		HasModel: gate != nil,
		Model:    gate,
		RenderOptions: answer.RenderOptions{
			Place:    cfg.SupportChannel,
			MaxRunes: cfg.ReplyMaxChars,
		},
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

func buildModelGate(cfg config.Config, logger *slog.Logger) (answer.Generator, answer.MaskingGenerator, *model.Gate, error) {
	if !cfg.HasModel() {
		return nil, nil, nil, nil
	}
	mCfg := model.Config{
		BaseURL:   cfg.ModelBaseURL,
		Alias:     cfg.ModelAlias,
		Policy:    cfg.ModelPolicy,
		Guardrail: cfg.ModelGuardrail,
		APIKey:    cfg.ModelKey,
		Timeout:   cfg.ModelTimeout,
	}
	mClient, err := model.New(mCfg, nil)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create model client: %w", err)
	}
	gateCfg := model.GateConfig{Endpoint: mClient, Logger: logger}
	if mClient.Masking() {
		gateCfg.Masker = mClient
	}
	gate, err := model.NewGate(gateCfg)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create model gate: %w", err)
	}
	var generator answer.Generator = gate
	var masker answer.MaskingGenerator
	if mClient.Masking() {
		masker = gate
	}
	return generator, masker, gate, nil
}

func buildRetriever(ctx context.Context, cfg config.Config, index *retrieval.Index, chunks []corpus.Chunk, logger *slog.Logger) (answer.Retriever, string, error) {
	if !cfg.HasEmbed() {
		logger.Info("retrieval mode", slog.String("mode", string(retrieval.ModeKeyword)))
		return index, "", nil
	}

	eCfg := embed.Config{
		BaseURL: cfg.EmbedBaseURL,
		Alias:   cfg.EmbedAlias,
		APIKey:  cfg.EmbedKey,
		Timeout: cfg.EmbedTimeout,
	}
	embClient, err := embed.New(eCfg, nil)
	if err != nil {
		return nil, "", fmt.Errorf("create embed client: %w", err)
	}

	minSim := retrieval.DefaultMinSimilarity
	if cfg.EmbedMinSimilarity > 0 {
		minSim = cfg.EmbedMinSimilarity
	}

	startCtx, cancel := context.WithTimeout(ctx, HybridStartTimeout)
	defer cancel()
	start := time.Now()
	hybrid, err := retrieval.NewHybrid(startCtx, index, chunks, embClient, minSim)
	if err != nil {
		return nil, "", fmt.Errorf("initialize hybrid retrieval: %w", err)
	}
	duration := time.Since(start)
	dim := hybrid.VectorDim()
	vecBytes := len(chunks) * dim * 4

	logger.Info("retrieval mode",
		slog.String("mode", string(retrieval.ModeHybrid)),
		slog.Int("chunk_count", len(chunks)),
		slog.Int("vector_dimension", dim),
		slog.Duration("duration", duration),
		slog.Int("approximate_vector_bytes", vecBytes),
	)
	return hybrid, embClient.Host(), nil
}

func buildPipeline(ctx context.Context, cfg config.Config, index *retrieval.Index, chunks []corpus.Chunk, logger *slog.Logger) (*answer.Pipeline, *model.Gate, error) {
	generator, masker, gate, err := buildModelGate(cfg, logger)
	if err != nil {
		return nil, nil, err
	}

	searcher, embedHost, err := buildRetriever(ctx, cfg, index, chunks, logger)
	if err != nil {
		return nil, nil, err
	}

	opts := retrieval.DefaultOptions()
	if cfg.TopPassages > 0 {
		opts.Limit = cfg.TopPassages
	}
	if cfg.MinScore > 0 {
		opts.MinScore = cfg.MinScore
	}

	ansCfg := answer.Config{
		Retriever:          searcher,
		Keyword:            index,
		EmbedHost:          embedHost,
		Generator:          generator,
		Masker:             masker,
		Options:            opts,
		Logger:             logger,
		MaxQuestionRunes:   cfg.QuestionMaxChars,
		MaxExcerptRunes:    cfg.ExcerptChars,
		SourceLinkTemplate: cfg.SourceLinkTemplate,
	}
	pipeline, err := answer.New(ansCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("create answer pipeline: %w", err)
	}
	return pipeline, gate, nil
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
