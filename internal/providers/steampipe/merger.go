package steampipe

import (
	"strings"
)

const managedMarker = "# managed-by: cfgctl"

// spcBlock represents a parsed block from an .spc file.
type spcBlock struct {
	// leading holds any lines before the block itself (blank lines, comments).
	leading string
	// content holds the connection block content (from `connection` to closing `}`).
	content string
	// name is the connection name extracted from the block header.
	name string
	// managed is true if the block is tagged with the managed marker.
	managed bool
}

// parseSPCBlocks splits an .spc file into discrete blocks, preserving all
// leading whitespace and comments. Blocks without a `connection` header are
// returned as a single leading fragment attached to the first real block or
// as a standalone block with an empty name.
func parseSPCBlocks(content string) []spcBlock {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(content, "\n")

	var blocks []spcBlock
	var leadingLines []string
	blockLines := make([]string, 0, 8)
	depth := 0
	inBlock := false

	flush := func() {
		if !inBlock {
			return
		}
		raw := strings.Join(blockLines, "\n")
		name := extractConnectionName(raw)
		leading := strings.Join(leadingLines, "\n")
		managed := isMarkedManaged(leadingLines)
		blocks = append(blocks, spcBlock{
			leading: leading,
			content: raw,
			name:    name,
			managed: managed,
		})
		leadingLines = nil
		blockLines = nil
		inBlock = false
		depth = 0
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if !inBlock {
			// Look for a connection block start.
			if strings.HasPrefix(trimmed, "connection ") && strings.HasSuffix(trimmed, "{") {
				inBlock = true
				depth = 1
				blockLines = []string{line}
				continue
			}
			leadingLines = append(leadingLines, line)
			continue
		}

		blockLines = append(blockLines, line)
		depth += strings.Count(line, "{") - strings.Count(line, "}")
		if depth <= 0 {
			flush()
		}
	}

	// Flush any trailing leading lines as a sentinel block (no connection).
	if len(leadingLines) > 0 {
		trailing := strings.TrimRight(strings.Join(leadingLines, "\n"), "\n")
		if trailing != "" {
			blocks = append(blocks, spcBlock{leading: trailing})
		}
	}

	return blocks
}

// isMarkedManaged returns true if any line in the leading slice equals the
// managed marker (ignoring surrounding whitespace).
func isMarkedManaged(leading []string) bool {
	for _, line := range leading {
		if strings.TrimSpace(line) == managedMarker {
			return true
		}
	}
	return false
}

// extractConnectionName parses the connection name from the block header line.
// E.g. `connection "aws_prod" {` → "aws_prod".
func extractConnectionName(block string) string {
	for _, line := range strings.Split(block, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "connection ") {
			continue
		}
		// connection "name" {
		rest := strings.TrimPrefix(trimmed, "connection ")
		rest = strings.TrimSpace(rest)
		if len(rest) < 2 || rest[0] != '"' {
			continue
		}
		end := strings.Index(rest[1:], "\"")
		if end < 0 {
			continue
		}
		return rest[1 : end+1]
	}
	return ""
}

// mergeBlocks merges user-managed blocks from existing with freshly generated
// managed blocks. The result is: user blocks in their original positions,
// followed by all new managed blocks.
//
// A user block is dropped when:
//   - its connection name exactly matches a generated block's name, or
//   - its AWS profile's account portion (before the first "/") normalises to
//     the same value as a generated block's profile account — meaning the
//     same account is now covered by cfgctl.
func mergeBlocks(existing []spcBlock, generated []spcBlock) []spcBlock {
	// Index generated blocks by connection name and by sanitized account name.
	generatedNames := make(map[string]bool, len(generated))
	generatedAccounts := make(map[string]bool, len(generated))
	for _, b := range generated {
		if b.name == "" {
			continue
		}
		generatedNames[b.name] = true
		if account := sanitizedProfileAccount(extractProfileValue(b.content)); account != "" {
			generatedAccounts[account] = true
		}
	}

	userBlocks := make([]spcBlock, 0, len(existing))
	for _, b := range existing {
		if b.name == "" {
			continue // sentinel / pure leading content
		}
		if b.managed {
			continue // stale managed block — replaced by generated set
		}
		// Aggregator blocks are always copied through unchanged — cfgctl does
		// not manage them and should never drop them.
		if isAggregatorBlock(b.content) {
			userBlocks = append(userBlocks, b)
			continue
		}
		if generatedNames[b.name] {
			continue // exact name collision
		}
		if account := sanitizedProfileAccount(extractProfileValue(b.content)); account != "" {
			if generatedAccounts[account] {
				continue // same AWS account now managed by cfgctl
			}
		}
		userBlocks = append(userBlocks, b)
	}

	return append(userBlocks, generated...)
}

// isAggregatorBlock reports whether a block contains type = "aggregator".
func isAggregatorBlock(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "type") {
			rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "type"))
			if strings.HasPrefix(rest, "=") {
				val := strings.TrimSpace(strings.TrimPrefix(rest, "="))
				val = strings.Trim(val, "\"")
				if val == "aggregator" {
					return true
				}
			}
		}
	}
	return false
}

// extractProfileValue parses the value of the profile field from a connection
// block's content. Returns empty string if not found.
func extractProfileValue(content string) string {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "profile") {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "profile"))
		if !strings.HasPrefix(rest, "=") {
			continue
		}
		rest = strings.TrimSpace(strings.TrimPrefix(rest, "="))
		return strings.Trim(rest, "\"")
	}
	return ""
}

// sanitizedProfileAccount returns the normalised account portion of an AWS
// profile name — the part before the first "/" — using the same rules as
// sanitizeProfileName. This lets us match "AFT-Management" against
// "aft-management/cloudinfra" (both normalise to "aft_management").
func sanitizedProfileAccount(profile string) string {
	account := profile
	if idx := strings.Index(profile, "/"); idx >= 0 {
		account = profile[:idx]
	}
	return sanitizeProfileName(account, "")
}

// renderBlocks serialises a block list back to file content, prefixed with the
// standard file header.
func renderBlocks(blocks []spcBlock) string {
	var sb strings.Builder
	sb.WriteString(fileHeader)
	sb.WriteString("\n")
	for i, b := range blocks {
		if b.content == "" {
			// Pure leading/trailing fragment.
			if b.leading != "" {
				sb.WriteString(b.leading)
				sb.WriteString("\n")
			}
			continue
		}

		leading := strings.TrimRight(b.leading, "\n")
		if leading != "" {
			sb.WriteString(leading)
			sb.WriteString("\n")
		}
		sb.WriteString(b.content)
		// Separate blocks with a blank line, but not after the last one.
		if i < len(blocks)-1 {
			sb.WriteString("\n\n")
		} else {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}
