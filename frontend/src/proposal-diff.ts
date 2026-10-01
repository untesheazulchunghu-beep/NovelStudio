export type ParagraphDiff = { kind: 'same' | 'added' | 'removed'; text: string }

function splitParagraphs(text: string): string[] {
  return text
    .replace(/\r\n/g, '\n')
    .split(/\n\s*\n/u)
    .map((paragraph) => paragraph.trim())
    .filter(Boolean)
}

export function diffParagraphs(currentText: string, proposedText: string): ParagraphDiff[] {
  const current = splitParagraphs(currentText)
  const proposed = splitParagraphs(proposedText)
  if (current.length * proposed.length > 40_000) {
    return [
      ...current.map((text) => ({ kind: 'removed' as const, text })),
      ...proposed.map((text) => ({ kind: 'added' as const, text })),
    ]
  }

  const lengths = Array.from({ length: current.length + 1 }, () => new Uint32Array(proposed.length + 1))
  for (let left = current.length - 1; left >= 0; left -= 1) {
    for (let right = proposed.length - 1; right >= 0; right -= 1) {
      lengths[left][right] =
        current[left] === proposed[right]
          ? lengths[left + 1][right + 1] + 1
          : Math.max(lengths[left + 1][right], lengths[left][right + 1])
    }
  }

  const result: ParagraphDiff[] = []
  let left = 0
  let right = 0
  while (left < current.length || right < proposed.length) {
    if (left < current.length && right < proposed.length && current[left] === proposed[right]) {
      result.push({ kind: 'same', text: current[left] })
      left += 1
      right += 1
    } else if (
      right < proposed.length &&
      (left === current.length || lengths[left][right + 1] > lengths[left + 1][right])
    ) {
      result.push({ kind: 'added', text: proposed[right] })
      right += 1
    } else if (left < current.length) {
      result.push({ kind: 'removed', text: current[left] })
      left += 1
    }
  }
  return result
}
