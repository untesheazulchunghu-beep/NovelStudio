import { Check, X } from 'lucide-react'
import type { Dispatch, SetStateAction } from 'react'
import type { Chapter, WorldAsset } from '../types'
import type { WorldAssetDialogDraft } from '../hooks/useWorldAssetDialogState'

const kindLabels: Record<string, string> = { place: '地点', organization: '组织', item: '物件', concept: '概念' }

export function WorldAssetDialog({ open, editing, editingId, draft, setDraft, chapters, assets, saving, error, onClose, onSave }: { open: boolean; editing: boolean; editingId: number | null; draft: WorldAssetDialogDraft; setDraft: Dispatch<SetStateAction<WorldAssetDialogDraft>>; chapters: Chapter[]; assets: WorldAsset[]; saving: boolean; error: string; onClose: () => void; onSave: () => void }) {
  if (!open) return null
  const toggle = (key: 'chapter_ids' | 'related_asset_ids', id: number) => setDraft((current) => ({ ...current, [key]: current[key].includes(id) ? current[key].filter((value) => value !== id) : [...current[key], id] }))
  return <div className="modal-backdrop" onMouseDown={(event) => { if (event.target === event.currentTarget && !saving) onClose() }}>
    <section className="project-dialog world-asset-dialog" role="dialog" aria-modal="true" aria-labelledby="world-asset-dialog-title">
      <header><div><span className="dialog-kicker">WORLD ASSET</span><h2 id="world-asset-dialog-title">{editing ? '编辑世界观资料' : '新建世界观资料'}</h2></div><button className="subtle-icon" onClick={onClose} disabled={saving} aria-label="关闭"><X size={16} /></button></header>
      <label>名称<input autoFocus value={draft.name} onChange={(event) => setDraft({ ...draft, name: event.target.value })} maxLength={120} placeholder="例如：雾港、银塔商会" disabled={saving} /></label>
      <label>类型<select value={draft.kind} onChange={(event) => setDraft({ ...draft, kind: event.target.value as WorldAssetDialogDraft['kind'] })} disabled={saving}>{Object.entries(kindLabels).map(([value, label]) => <option value={value} key={value}>{label}</option>)}</select></label>
      <label>简介<textarea value={draft.description} onChange={(event) => setDraft({ ...draft, description: event.target.value })} rows={3} maxLength={1600} placeholder="一句话说明它在故事中的作用。" disabled={saving} /></label>
      <label>补充设定<textarea value={draft.details} onChange={(event) => setDraft({ ...draft, details: event.target.value })} rows={4} maxLength={2400} placeholder="记录规则、历史、外观或使用限制。" disabled={saving} /></label>
      <div className="world-asset-checks"><fieldset><legend>关联章节</legend>{chapters.length === 0 && <small>还没有章节</small>}{chapters.map((chapter) => <label key={chapter.id}><input type="checkbox" checked={draft.chapter_ids.includes(chapter.id)} onChange={() => toggle('chapter_ids', chapter.id)} disabled={saving} />第{chapter.number}章 · {chapter.title}</label>)}</fieldset><fieldset><legend>关联资料</legend>{assets.filter((asset) => asset.id !== editingId).map((asset) => <label key={asset.id}><input type="checkbox" checked={draft.related_asset_ids.includes(asset.id)} onChange={() => toggle('related_asset_ids', asset.id)} disabled={saving} />{asset.name} · {kindLabels[asset.kind] ?? asset.kind}</label>)}</fieldset></div>
      {error && <p className="dialog-error" role="alert">{error}</p>}
      <div className="dialog-actions"><button className="toolbar-button" onClick={onClose} disabled={saving}>取消</button><button className="primary-button" onClick={onSave} disabled={!draft.name.trim() || saving}><Check size={14} /> {saving ? '正在保存…' : editing ? '保存修改' : '添加资料'}</button></div>
    </section>
  </div>
}
