import { Check, X } from 'lucide-react'
import type { Dispatch, SetStateAction } from 'react'
import type { Chapter } from '../types'
import type { TimelineDialogDraft } from '../hooks/useTimelineDialogState'

export function TimelineDialog({ open, editing, draft, setDraft, chapters, saving, error, onClose, onSave }: { open: boolean; editing: boolean; draft: TimelineDialogDraft; setDraft: Dispatch<SetStateAction<TimelineDialogDraft>>; chapters: Chapter[]; saving: boolean; error: string; onClose: () => void; onSave: () => void }) {
  if (!open) return null
  return <div className="modal-backdrop" onMouseDown={(event) => { if (event.target === event.currentTarget && !saving) onClose() }}>
    <section className="project-dialog" role="dialog" aria-modal="true" aria-labelledby="timeline-dialog-title">
      <header><div><span className="dialog-kicker">STORY TIMELINE</span><h2 id="timeline-dialog-title">{editing ? '编辑时间线事件' : '新建时间线事件'}</h2></div><button className="subtle-icon" onClick={onClose} aria-label="关闭" disabled={saving}><X size={16} /></button></header>
      <label>事件标题<input autoFocus value={draft.title} onChange={(event) => setDraft({ ...draft, title: event.target.value })} placeholder="例如：林澈发现信件来自未来" maxLength={120} disabled={saving} /></label>
      <label>故事时间<input value={draft.time_label} onChange={(event) => setDraft({ ...draft, time_label: event.target.value })} placeholder="例如：第三天清晨、潮汐最低时" maxLength={80} disabled={saving} /></label>
      <label>状态<select value={draft.status} onChange={(event) => setDraft({ ...draft, status: event.target.value as TimelineDialogDraft['status'] })} disabled={saving}><option value="planned">计划</option><option value="active">进行中</option><option value="done">已完成</option></select></label>
      <label>关联章节<select value={draft.chapter_id} onChange={(event) => setDraft({ ...draft, chapter_id: Number(event.target.value) || 0 })} disabled={saving}><option value={0}>暂不关联</option>{chapters.map((chapter) => <option value={chapter.id} key={chapter.id}>第 {chapter.number} 章 · {chapter.title}</option>)}</select></label>
      <label>事件说明<textarea value={draft.description} onChange={(event) => setDraft({ ...draft, description: event.target.value })} placeholder="记录这件事如何改变人物、线索或局面。" rows={4} maxLength={2000} disabled={saving} /></label>
      {error && <p className="dialog-error" role="alert">{error}</p>}
      <div className="dialog-actions"><button className="toolbar-button" onClick={onClose} disabled={saving}>取消</button><button className="primary-button" onClick={onSave} disabled={!draft.title.trim() || saving}><Check size={14} /> {saving ? '正在保存…' : editing ? '保存修改' : '添加事件'}</button></div>
    </section>
  </div>
}
