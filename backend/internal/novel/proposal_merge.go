package novel

import (
	"sort"
	"strings"
)

type proposalParagraph struct {
	text       string
	start, end int
}

type proposalDiffParagraph struct {
	kind            string
	text            string
	hunk            int
	currentIndex    int
	proposedIndex   int
	currentBoundary int
}

type proposalHunk struct {
	current      []int
	proposed     []int
	currentIndex int
}

type proposalPatch struct {
	start, end int
	text       string
}

func proposalParagraphs(text string) []proposalParagraph {
	paragraphs := make([]proposalParagraph, 0)
	start, end := -1, -1
	flush := func() {
		if start >= 0 {
			raw := text[start:end]
			paragraph := strings.TrimSpace(strings.ReplaceAll(raw, "\r\n", "\n"))
			if paragraph != "" {
				paragraphs = append(paragraphs, proposalParagraph{text: paragraph, start: start, end: end})
			}
		}
		start, end = -1, -1
	}

	for lineStart := 0; lineStart < len(text); {
		lineEnd := strings.IndexByte(text[lineStart:], '\n')
		hasNewline := lineEnd >= 0
		if hasNewline {
			lineEnd += lineStart
		} else {
			lineEnd = len(text)
		}
		contentEnd := lineEnd
		if hasNewline && contentEnd > lineStart && text[contentEnd-1] == '\r' {
			contentEnd--
		}
		line := text[lineStart:contentEnd]
		if strings.TrimSpace(line) == "" {
			flush()
		} else {
			if start < 0 {
				start = lineStart
			}
			end = contentEnd
		}
		if hasNewline {
			lineStart = lineEnd + 1
		} else {
			lineStart = len(text)
		}
	}
	flush()
	return paragraphs
}

func proposalParagraphDiff(current, proposed []proposalParagraph) ([]proposalDiffParagraph, int) {
	result := make([]proposalDiffParagraph, 0, len(current)+len(proposed))
	hunkCount := 0
	changed := false
	appendParagraph := func(kind, text string, currentIndex, proposedIndex, boundary int) {
		hunk := -1
		if kind != "same" {
			if !changed {
				hunkCount++
			}
			hunk = hunkCount - 1
			changed = true
		} else {
			changed = false
		}
		result = append(result, proposalDiffParagraph{
			kind: kind, text: text, hunk: hunk,
			currentIndex: currentIndex, proposedIndex: proposedIndex, currentBoundary: boundary,
		})
	}

	if len(current)*len(proposed) > 40_000 {
		for index, paragraph := range current {
			appendParagraph("removed", paragraph.text, index, -1, index)
		}
		for index, paragraph := range proposed {
			appendParagraph("added", paragraph.text, -1, index, len(current))
		}
		return result, hunkCount
	}

	lengths := make([][]uint32, len(current)+1)
	for index := range lengths {
		lengths[index] = make([]uint32, len(proposed)+1)
	}
	for left := len(current) - 1; left >= 0; left-- {
		for right := len(proposed) - 1; right >= 0; right-- {
			if current[left].text == proposed[right].text {
				lengths[left][right] = lengths[left+1][right+1] + 1
			} else if lengths[left+1][right] > lengths[left][right+1] {
				lengths[left][right] = lengths[left+1][right]
			} else {
				lengths[left][right] = lengths[left][right+1]
			}
		}
	}

	left, right := 0, 0
	for left < len(current) || right < len(proposed) {
		if left < len(current) && right < len(proposed) && current[left].text == proposed[right].text {
			appendParagraph("same", current[left].text, left, right, left)
			left++
			right++
		} else if right < len(proposed) && (left == len(current) || lengths[left][right+1] > lengths[left+1][right]) {
			appendParagraph("added", proposed[right].text, -1, right, left)
			right++
		} else if left < len(current) {
			appendParagraph("removed", current[left].text, left, -1, left)
			left++
		}
	}
	return result, hunkCount
}

func mergeProposalHunks(currentText, proposedText string, selected []int) (string, int, bool) {
	current := proposalParagraphs(currentText)
	proposed := proposalParagraphs(proposedText)
	diff, hunkCount := proposalParagraphDiff(current, proposed)
	selection := make(map[int]struct{}, len(selected))
	for _, index := range selected {
		if index < 0 || index >= hunkCount {
			return "", hunkCount, false
		}
		if _, duplicate := selection[index]; duplicate {
			return "", hunkCount, false
		}
		selection[index] = struct{}{}
	}

	hunks := make([]proposalHunk, hunkCount)
	for _, paragraph := range diff {
		if paragraph.hunk < 0 {
			continue
		}
		hunk := &hunks[paragraph.hunk]
		hunk.currentIndex = paragraph.currentBoundary
		if paragraph.kind == "removed" {
			hunk.current = append(hunk.current, paragraph.currentIndex)
		} else {
			hunk.proposed = append(hunk.proposed, paragraph.proposedIndex)
		}
	}

	separator := "\n\n"
	if strings.Contains(currentText, "\r\n") {
		separator = "\r\n\r\n"
	}
	patches := make([]proposalPatch, 0, len(selection))
	for index := range selection {
		hunk := hunks[index]
		newParagraphs := make([]string, 0, len(hunk.proposed))
		for _, proposedIndex := range hunk.proposed {
			paragraph := proposed[proposedIndex]
			newParagraphs = append(newParagraphs, paragraphRawText(proposedText, paragraph))
		}
		replacement := strings.Join(newParagraphs, separator)
		patch := proposalPatch{text: replacement}
		if len(hunk.current) > 0 {
			first, last := hunk.current[0], hunk.current[len(hunk.current)-1]
			patch.start = current[first].start
			patch.end = current[last].end
			if len(newParagraphs) == 0 {
				if last+1 < len(current) {
					patch.end = current[last+1].start
				} else if first > 0 {
					patch.start = current[first-1].end
				}
			}
		} else if len(current) == 0 {
			patch.start = 0
			patch.end = len(currentText)
		} else if hunk.currentIndex < len(current) {
			patch.start = current[hunk.currentIndex].start
			patch.end = patch.start
			patch.text += separator
		} else {
			patch.start = current[len(current)-1].end
			patch.end = patch.start
			patch.text = separator + patch.text
		}
		patches = append(patches, patch)
	}

	sort.Slice(patches, func(left, right int) bool {
		if patches[left].start == patches[right].start {
			return patches[left].end > patches[right].end
		}
		return patches[left].start > patches[right].start
	})
	merged := currentText
	for _, patch := range patches {
		merged = merged[:patch.start] + patch.text + merged[patch.end:]
	}
	return merged, hunkCount, true
}

func paragraphRawText(text string, paragraph proposalParagraph) string {
	return text[paragraph.start:paragraph.end]
}
