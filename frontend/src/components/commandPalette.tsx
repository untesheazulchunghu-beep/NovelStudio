import { CalendarClock, Command, FileText, Globe2, Lightbulb, Search, Users, X } from 'lucide-react'
import type { KeyboardEvent, RefObject } from 'react'
import { isIMEComposing } from '../lib/text'
import type { PaletteResult } from '../workbenchTypes'

interface CommandPaletteProps {
  query: string
  results: PaletteResult[]
  selectedIndex: number
  inputRef: RefObject<HTMLInputElement | null>
  activeResultRef: RefObject<HTMLButtonElement | null>
  onQueryChange: (query: string) => void
  onSelectIndex: (index: number) => void
  onActivate: (result: PaletteResult) => void
  onClose: () => void
}

export function CommandPalette({
  query,
  results,
  selectedIndex,
  inputRef,
  activeResultRef,
  onQueryChange,
  onSelectIndex,
  onActivate,
  onClose,
}: CommandPaletteProps) {
  const activeResult = results[selectedIndex]

  const handleKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (isIMEComposing(event.nativeEvent)) return
    if (event.key === 'ArrowDown' && results.length > 0) {
      event.preventDefault()
      onSelectIndex(Math.min(selectedIndex + 1, results.length - 1))
    } else if (event.key === 'ArrowUp' && results.length > 0) {
      event.preventDefault()
      onSelectIndex(Math.max(selectedIndex - 1, 0))
    } else if (event.key === 'Enter' && activeResult && !activeResult.disabled) {
      event.preventDefault()
      onActivate(activeResult)
    } else if (event.key === 'Escape') {
      event.preventDefault()
      onClose()
    }
  }

  return (
    <div className="modal-backdrop search-backdrop" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose() }}>
      <section className="command-palette" role="dialog" aria-modal="true" aria-labelledby="command-palette-title">
        <div className="command-palette-input-row">
          <Search size={17} />
          <input
            ref={inputRef}
            autoFocus
            role="combobox"
            aria-expanded="true"
            aria-controls="command-palette-results"
            aria-activedescendant={activeResult ? `palette-${activeResult.id}` : undefined}
            aria-autocomplete="list"
            aria-label="搜索当前项目"
            placeholder="搜索章节、角色、便笺、时间线、世界观资料或命令…"
            value={query}
            onChange={(event) => onQueryChange(event.target.value)}
            onKeyDown={handleKeyDown}
          />
          <kbd>ESC</kbd>
          <button className="subtle-icon" onClick={onClose} aria-label="关闭搜索"><X size={15} /></button>
        </div>
        <h2 id="command-palette-title" className="sr-only">搜索当前项目</h2>
        <div className="command-palette-results" id="command-palette-results" role="listbox" aria-label="搜索结果">
          {results.map((result, index) => {
            const ResultIcon = result.kind === 'chapter' ? FileText : result.kind === 'character' ? Users : result.kind === 'note' ? Lightbulb : result.kind === 'timeline' ? CalendarClock : result.kind === 'world-asset' ? Globe2 : Command
            const kindLabel = result.kind === 'chapter' ? '\u7ae0\u8282' : result.kind === 'character' ? '\u89d2\u8272' : result.kind === 'note' ? '\u4fbf\u7b8a' : result.kind === 'timeline' ? '\u65f6\u95f4\u7ebf' : result.kind === 'world-asset' ? '\u4e16\u754c\u89c2\u8d44\u6599' : '\u547d\u4ee4'
            return (
              <button
                key={result.id}
                id={`palette-${result.id}`}
                ref={index === selectedIndex ? activeResultRef : undefined}
                className={`palette-result ${index === selectedIndex ? 'active' : ''}`}
                role="option"
                aria-selected={index === selectedIndex}
                disabled={result.disabled}
                onMouseEnter={() => onSelectIndex(index)}
                onClick={() => onActivate(result)}
              >
                <span className={`palette-result-icon ${result.kind}`}><ResultIcon size={14} /></span>
                <span className="palette-result-copy"><strong>{result.title}</strong><small>{result.detail}</small></span>
                <span className="palette-result-kind">{kindLabel}</span>
              </button>
            )
          })}
          {results.length === 0 && <div className="palette-empty"><Search size={16} /><span>{query.trim() ? '没有找到匹配内容' : '当前项目还没有可搜索的内容'}</span></div>}
        </div>
        <footer className="palette-footer"><span><kbd>↑</kbd><kbd>↓</kbd> 选择</span><span><kbd>Enter</kbd> 打开</span><span><kbd>Esc</kbd> 关闭</span></footer>
      </section>
    </div>
  )
}
