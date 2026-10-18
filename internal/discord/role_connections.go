package discord

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
)

// PushRoleConnections registers application role connection metadata records with Discord via REST.
func PushRoleConnections(
	ctx context.Context,
	token string,
	records []discord.ApplicationRoleConnectionMetadata,
	out io.Writer,
) error {
	return pushRoleConnections(ctx, token, records, out, clientOptions{})
}

func pushRoleConnections(
	ctx context.Context,
	token string,
	records []discord.ApplicationRoleConnectionMetadata,
	out io.Writer,
	opts clientOptions,
) error {
	if token == "" {
		return errors.New("bot token is required")
	}

	client, err := newRestClient(token, opts.restURL)
	if err != nil {
		return err
	}
	defer client.Close(ctx)

	appID, err := lookupApplicationID(ctx, client.Rest, token, opts.lookupDeadline())
	if err != nil {
		return err
	}

	updated, err := client.Rest.UpdateApplicationRoleConnectionMetadata(appID, records, rest.WithCtx(ctx))
	if err != nil {
		return callError("update application role connection metadata", err, token)
	}

	if err := verifyReturnedMetadata(records, updated); err != nil {
		return err
	}

	return printPushedRecords(records, out)
}

func verifyReturnedMetadata(
	want []discord.ApplicationRoleConnectionMetadata,
	got []discord.ApplicationRoleConnectionMetadata,
) error {
	if len(got) != len(want) {
		return fmt.Errorf("role connection metadata mismatch: count got %d, want %d", len(got), len(want))
	}
	for i, w := range want {
		g := got[i]
		if g.Type != w.Type || g.Key != w.Key || g.Name != w.Name || g.Description != w.Description ||
			!equalLocalizations(g.NameLocalizations, w.NameLocalizations) ||
			!equalLocalizations(g.DescriptionLocalizations, w.DescriptionLocalizations) {
			return fmt.Errorf("role connection metadata mismatch for record %q", w.Key)
		}
	}
	return nil
}

func equalLocalizations(a, b map[discord.Locale]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func printPushedRecords(records []discord.ApplicationRoleConnectionMetadata, out io.Writer) error {
	for _, r := range records {
		if _, err := fmt.Fprintf(out, "pushed %s type=%d\n", r.Key, r.Type); err != nil {
			return fmt.Errorf("write pushed record: %w", err)
		}
	}
	return nil
}
