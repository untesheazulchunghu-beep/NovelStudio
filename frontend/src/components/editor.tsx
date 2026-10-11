import { PenLine, Plus } from 'lucide-react'

export function EmptyEditor({ onCreate, disabled }: { onCreate: () => void; disabled: boolean }) {
  return <div className="empty-editor"><div className="empty-orb"><PenLine size={23} /></div><h2>准备好写下一章了吗？</h2><p>从一个章节开始，Agent 会帮你管理线索、角色与节奏。</p><button className="primary-button" onClick={onCreate} disabled={disabled}><Plus size={15} /> 新建第一章</button></div>
}

export function GitBranchIcon() { return <span className="status-glyph">⑂</span> }

export function CircleStatus() { return <span className="status-glyph green-glyph">●</span> }
