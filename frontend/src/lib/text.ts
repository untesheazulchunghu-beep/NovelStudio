export const hanScriptPattern = /\p{Script=Han}/u

export const wordCharacterPattern = /[\p{L}\p{M}\p{N}_]/u

export function normalizeEditorLineEndings(text: string) {
  return text.replace(/\r\n?/g, '\n')
}

export function countUnicodeCharacters(text: string) {
  return Array.from(text).length
}

export function countUnicodeCharactersInRange(text: string, start: number, end: number, limit: number) {
  let count = 0
  for (let index = start; index < end;) {
    const codePoint = text.codePointAt(index)
    if (codePoint === undefined) break
    index += codePoint > 0xffff ? 2 : 1
    count += 1
    if (count > limit) return count
  }
  return count
}

export function sliceUnicodeRange(text: string, start: number, end: number, limit: number) {
  let index = start
  let count = 0
  while (index < end && count < limit) {
    const codePoint = text.codePointAt(index)
    if (codePoint === undefined) break
    index += codePoint > 0xffff ? 2 : 1
    count += 1
  }
  return text.slice(start, Math.min(index, end))
}

export function sliceUnicodeTail(text: string, limit: number) {
  let start = text.length
  let count = 0
  while (start > 0 && count < limit) {
    start -= 1
    const current = text.charCodeAt(start)
    if (current >= 0xdc00 && current <= 0xdfff && start > 0) {
      const previous = text.charCodeAt(start - 1)
      if (previous >= 0xd800 && previous <= 0xdbff) start -= 1
    }
    count += 1
  }
  return text.slice(start)
}

export function sliceUnicodeHead(text: string, limit: number) {
  let end = 0
  let count = 0
  for (const character of text) {
    if (count >= limit) break
    end += character.length
    count += 1
  }
  return text.slice(0, end)
}

export function normalizeUnicodeSelection(text: string, start: number, end: number) {
  let safeStart = start
  let safeEnd = end
  if (safeStart > 0 && safeStart < text.length) {
    const previous = text.charCodeAt(safeStart - 1)
    const current = text.charCodeAt(safeStart)
    if (previous >= 0xd800 && previous <= 0xdbff && current >= 0xdc00 && current <= 0xdfff) safeStart -= 1
  }
  if (safeEnd > 0 && safeEnd < text.length) {
    const previous = text.charCodeAt(safeEnd - 1)
    const current = text.charCodeAt(safeEnd)
    if (previous >= 0xd800 && previous <= 0xdbff && current >= 0xdc00 && current <= 0xdfff) safeEnd += 1
  }
  return { start: safeStart, end: safeEnd }
}

export function contextAroundSelection(text: string, start: number, end: number, limit = 400) {
  if (start < 0 || start >= end || end > text.length) return ""
  let beforeStart = start
  let beforeCount = 0
  while (beforeStart > 0 && beforeCount < limit) {
    const currentIndex = beforeStart - 1
    const current = text.charCodeAt(currentIndex)
    if (current >= 0xdc00 && current <= 0xdfff && currentIndex > 0) {
      const previous = text.charCodeAt(currentIndex - 1)
      beforeStart = previous >= 0xd800 && previous <= 0xdbff ? currentIndex - 1 : currentIndex
    } else {
      beforeStart = currentIndex
    }
    beforeCount += 1
  }
  let afterEnd = end
  let afterCount = 0
  while (afterEnd < text.length && afterCount < limit) {
    const codePoint = text.codePointAt(afterEnd)
    if (codePoint === undefined) break
    afterEnd += codePoint > 0xffff ? 2 : 1
    afterCount += 1
  }
  const before = sliceUnicodeRange(text, beforeStart, start, limit)
  const after = sliceUnicodeRange(text, end, afterEnd, limit)
  const parts = []
  if (before) parts.push("选区前文：" + (beforeStart > 0 ? "…" : "") + before)
  if (after) parts.push("选区后文：" + after + (afterEnd < text.length ? "…" : ""))
  return parts.join("\n")
}

export function isIMEComposing(event: KeyboardEvent) {
  return event.isComposing || event.keyCode === 229
}

export function countTextWords(text: string) {
  let count = 0
  let inWord = false
  const flush = () => {
    if (!inWord) return
    count += 1
    inWord = false
  }
  for (const character of text) {
    if (hanScriptPattern.test(character)) {
      flush()
      count += 1
    } else if (wordCharacterPattern.test(character)) {
      inWord = true
    } else {
      flush()
    }
  }
  flush()
  return count
}
