// 字数统计 counts characters after removing whitespace.
export function countManuscriptCharacters(text: string) {
  return text.replace(/\s/g, '').length
}
