package ai

// diffHunk describes one difference between an original field text and the
// revised text proposed by the model. Positions are rune offsets so that the
// server and the browser address the same characters.
type diffHunk struct {
	OriginalStart int
	OriginalEnd   int
	RevisedStart  int
	RevisedEnd    int
	Original      string
	Revised       string
}

// isInsertion reports whether the hunk only adds content that the original
// text does not contain.
func (hunk diffHunk) isInsertion() bool {
	return hunk.OriginalStart == hunk.OriginalEnd && hunk.RevisedEnd > hunk.RevisedStart
}

// isDeletion reports whether the hunk only removes content.
func (hunk diffHunk) isDeletion() bool {
	return hunk.RevisedStart == hunk.RevisedEnd && hunk.OriginalEnd > hunk.OriginalStart
}

const (
	// maxDiffCells caps the dynamic programming table. Field texts are far
	// smaller than this, so the cap only guards against pathological input.
	maxDiffCells = 4_000_000
	// diffMergeGap is the number of unchanged runes that may separate two
	// changes before they are treated as one edit.
	diffMergeGap = 4
)

// diffText computes the differences between the original and the revised text.
// It trims the common prefix and suffix first and then runs an LCS diff over
// the remaining middle, so unchanged lead-ins and tails never produce hunks.
func diffText(original, revised string) []diffHunk {
	originalRunes := []rune(original)
	revisedRunes := []rune(revised)

	prefix := 0
	for prefix < len(originalRunes) && prefix < len(revisedRunes) && originalRunes[prefix] == revisedRunes[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(originalRunes)-prefix && suffix < len(revisedRunes)-prefix &&
		originalRunes[len(originalRunes)-1-suffix] == revisedRunes[len(revisedRunes)-1-suffix] {
		suffix++
	}

	middleOriginal := originalRunes[prefix : len(originalRunes)-suffix]
	middleRevised := revisedRunes[prefix : len(revisedRunes)-suffix]

	hunks := lcsHunks(middleOriginal, middleRevised)
	for index := range hunks {
		hunks[index].OriginalStart += prefix
		hunks[index].OriginalEnd += prefix
		hunks[index].RevisedStart += prefix
		hunks[index].RevisedEnd += prefix
	}
	return hunks
}

// lcsHunks diffs two rune slices and returns the changed regions in order.
func lcsHunks(original, revised []rune) []diffHunk {
	originalLength := len(original)
	revisedLength := len(revised)

	switch {
	case originalLength == 0 && revisedLength == 0:
		return nil
	case originalLength == 0:
		return []diffHunk{{RevisedStart: 0, RevisedEnd: revisedLength, Revised: string(revised)}}
	case revisedLength == 0:
		return []diffHunk{{OriginalStart: 0, OriginalEnd: originalLength, Original: string(original)}}
	case originalLength*revisedLength > maxDiffCells:
		return []diffHunk{{
			OriginalStart: 0, OriginalEnd: originalLength,
			RevisedStart: 0, RevisedEnd: revisedLength,
			Original: string(original), Revised: string(revised),
		}}
	}

	table := make([][]int, originalLength+1)
	for index := range table {
		table[index] = make([]int, revisedLength+1)
	}
	for i := originalLength - 1; i >= 0; i-- {
		for j := revisedLength - 1; j >= 0; j-- {
			switch {
			case original[i] == revised[j]:
				table[i][j] = table[i+1][j+1] + 1
			case table[i+1][j] >= table[i][j+1]:
				table[i][j] = table[i+1][j]
			default:
				table[i][j] = table[i][j+1]
			}
		}
	}

	var hunks []diffHunk
	i, j := 0, 0
	for i < originalLength && j < revisedLength {
		if original[i] == revised[j] {
			i++
			j++
			continue
		}
		originalStart, revisedStart := i, j
		for i < originalLength && j < revisedLength && original[i] != revised[j] {
			if table[i+1][j] >= table[i][j+1] {
				i++
			} else {
				j++
			}
		}
		hunks = append(hunks, diffHunk{
			OriginalStart: originalStart, OriginalEnd: i,
			RevisedStart: revisedStart, RevisedEnd: j,
			Original: string(original[originalStart:i]), Revised: string(revised[revisedStart:j]),
		})
	}
	if i < originalLength {
		hunks = append(hunks, diffHunk{
			OriginalStart: i, OriginalEnd: originalLength,
			RevisedStart: revisedLength, RevisedEnd: revisedLength,
			Original: string(original[i:]),
		})
	}
	if j < revisedLength {
		hunks = append(hunks, diffHunk{
			OriginalStart: originalLength, OriginalEnd: originalLength,
			RevisedStart: j, RevisedEnd: revisedLength,
			Revised: string(revised[j:]),
		})
	}
	return mergeHunks(original, revised, hunks)
}

// mergeHunks turns adjacent deletions and insertions into replacements and
// joins changes that are separated by only a few unchanged runes, so that one
// rewritten phrase becomes one readable suggestion.
func mergeHunks(original, revised []rune, hunks []diffHunk) []diffHunk {
	paired := make([]diffHunk, 0, len(hunks))
	for index := 0; index < len(hunks); index++ {
		current := hunks[index]
		if index+1 < len(hunks) {
			next := hunks[index+1]
			if (current.isDeletion() && next.isInsertion()) || (current.isInsertion() && next.isDeletion()) {
				merged := diffHunk{
					OriginalStart: minInt(current.OriginalStart, next.OriginalStart),
					OriginalEnd:   maxInt(current.OriginalEnd, next.OriginalEnd),
					RevisedStart:  minInt(current.RevisedStart, next.RevisedStart),
					RevisedEnd:    maxInt(current.RevisedEnd, next.RevisedEnd),
				}
				merged.Original = string(original[merged.OriginalStart:merged.OriginalEnd])
				merged.Revised = string(revised[merged.RevisedStart:merged.RevisedEnd])
				paired = append(paired, merged)
				index++
				continue
			}
		}
		paired = append(paired, current)
	}

	merged := make([]diffHunk, 0, len(paired))
	for _, hunk := range paired {
		if len(merged) > 0 {
			last := &merged[len(merged)-1]
			gap := hunk.OriginalStart - last.OriginalEnd
			if gap >= 0 && gap <= diffMergeGap {
				last.OriginalEnd = hunk.OriginalEnd
				last.RevisedEnd = hunk.RevisedEnd
				last.Original = string(original[last.OriginalStart:last.OriginalEnd])
				last.Revised = string(revised[last.RevisedStart:last.RevisedEnd])
				continue
			}
		}
		merged = append(merged, hunk)
	}
	return merged
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
