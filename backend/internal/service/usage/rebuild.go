package usage

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

// Rebuild re-derives the whole ledger from the persisted chat event logs and
// replaces the monthly files with the result.
//
// It is idempotent: a run is keyed by (chatId, event timestamp), which is the
// same pair a live record carries, so re-running produces byte-identical
// files. Attribution that only exists on live records — the acting user, the
// run id, and the scheduled flag — is carried over from the current ledger
// wherever a key matches. Runs that were never recorded live therefore come
// back without a user, because chat event logs do not store who typed the
// prompt.
func (s *Service) Rebuild(ctx context.Context) (RebuildResult, error) {
	if s == nil || s.repo == nil {
		return RebuildResult{}, ErrUnavailable
	}
	if s.chats == nil {
		return RebuildResult{}, errors.New("usage rebuild needs a chat directory")
	}

	existing, err := s.existingByKey(ctx)
	if err != nil {
		return RebuildResult{}, err
	}
	prices, err := s.repo.Prices(ctx)
	if err != nil {
		return RebuildResult{}, err
	}
	chats, err := s.chats.List(ctx)
	if err != nil {
		return RebuildResult{}, err
	}

	slugs := s.projectSlugs(ctx)
	result := RebuildResult{}
	records := make([]Record, 0, len(chats))
	for _, chat := range chats {
		if err := ctx.Err(); err != nil {
			return RebuildResult{}, err
		}
		result.Chats++
		events, err := s.chats.ReadEvents(ctx, chat.ID)
		if err != nil {
			return RebuildResult{}, fmt.Errorf("read events for chat %s: %w", chat.ID, err)
		}
		for _, event := range events {
			prior, found := existing[recordKey(string(chat.ID), event.T)]
			// Live ledger records carry provider/model attribution that old chat
			// events did not persist. Prefer it before interpreting legacy usage.
			effectiveChat := chat
			if found {
				if event.Provider == "" && prior.Provider != "" {
					event.Provider = servicechat.Provider(prior.Provider)
				}
				if prior.Model != "" {
					effectiveChat.Model = prior.Model
				}
			}
			record, ok := recordFromChatEvent(effectiveChat, event, slugs, prices)
			if !ok {
				continue
			}
			if found {
				record.RunID = prior.RunID
				record.UserEmail = prior.UserEmail
				record.Scheduled = prior.Scheduled
				if prior.UserEmail != "" {
					result.PreservedActors++
				}
			}
			records = append(records, record)
		}
	}

	sort.SliceStable(records, func(i, j int) bool {
		if records[i].At != records[j].At {
			return records[i].At < records[j].At
		}
		return records[i].ChatID < records[j].ChatID
	})
	months, err := s.repo.ReplaceAll(ctx, records)
	if err != nil {
		return RebuildResult{}, err
	}
	result.Records = len(records)
	result.Months = months
	return result, nil
}

// recordFromChatEvent maps one persisted `complete` event onto a ledger
// record. Only completed runs are counted; a failed run bills nothing the
// platform can observe.
func recordFromChatEvent(
	chat servicechat.Meta,
	event servicechat.Event,
	slugs map[string]string,
	prices PriceTable,
) (Record, bool) {
	if event.Type != "complete" || event.T <= 0 {
		return Record{}, false
	}
	provider := strings.TrimSpace(string(event.Provider))
	if provider == "" {
		provider = strings.TrimSpace(string(servicechat.NormalizeProvider(chat.Provider)))
	}
	usage, hasUsage := agent.ParseUsage(event.Usage)
	// Before the normalized usage schema was versioned, Remote persisted
	// Codex's inclusive input_tokens beside cache_read_input_tokens. Convert
	// that legacy shape once during rebuild. This provider-specific branch is
	// migration code only; live adapters emit the provider-neutral contract.
	if provider == string(agent.ProviderCodex) && usage.SchemaVersion < agent.UsageSchemaVersion {
		usage = agent.NormalizeInclusiveInput(usage)
	}
	model := strings.TrimSpace(usage.Model)
	if model == "" {
		model = strings.TrimSpace(chat.Model)
	}

	record := Record{
		At:               event.T,
		ProjectID:        string(chat.ProjectID),
		ProjectSlug:      slugs[string(chat.ProjectID)],
		ChatID:           string(chat.ID),
		RunID:            fmt.Sprintf("%s-%d", chat.ID, event.Seq),
		Provider:         provider,
		Model:            model,
		InputTokens:      usage.InputTokens,
		OutputTokens:     usage.OutputTokens,
		CacheReadTokens:  usage.CacheReadTokens,
		CacheWriteTokens: usage.CacheWriteTokens,
		DurationMs:       usage.DurationMs,
		Turns:            usage.Turns,
	}
	if !hasUsage && record.Provider == "" {
		return Record{}, false
	}
	if usage.CostUSD != nil {
		cost := *usage.CostUSD
		record.CostUSD = &cost
	} else if estimate, ok := prices.Estimate(
		record.Model,
		record.InputTokens,
		record.OutputTokens,
		record.CacheReadTokens,
		record.CacheWriteTokens,
	); ok {
		record.CostUSD = &estimate
		record.Estimated = true
	}
	return record, true
}

// existingByKey indexes the current ledger so a rebuild can preserve the
// attribution only live recording knows about.
func (s *Service) existingByKey(ctx context.Context) (map[string]Record, error) {
	index := map[string]Record{}
	err := s.repo.Scan(ctx, 0, 0, func(record Record) bool {
		index[recordKey(record.ChatID, record.At)] = record
		return true
	})
	if err != nil {
		return nil, err
	}
	return index, nil
}

func (s *Service) projectSlugs(ctx context.Context) map[string]string {
	slugs := map[string]string{}
	if s.projects == nil {
		return slugs
	}
	metas, err := s.projects.ListVisible(ctx, "", true)
	if err != nil {
		return slugs
	}
	for _, meta := range metas {
		slugs[string(meta.ID)] = meta.Slug
	}
	return slugs
}

func recordKey(chatID string, at int64) string {
	return fmt.Sprintf("%s|%d", chatID, at)
}
