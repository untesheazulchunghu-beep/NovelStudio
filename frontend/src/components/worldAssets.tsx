import { ArrowRight, Globe2, Network, Pencil, Plus, Rows3, Trash2 } from 'lucide-react'
import { useMemo, useState } from 'react'
import type { Chapter, WorldAsset } from '../types'

const kindLabels: Record<string, string> = {
  place: '\u5730\u70b9',
  organization: '\u7ec4\u7ec7',
  item: '\u7269\u4ef6',
  concept: '\u6982\u5ff5',
}

const text = {
  title: '\u4e16\u754c\u89c2\u8d44\u6599',
  intro: '\u6574\u7406\u5730\u70b9\u3001\u7ec4\u7ec7\u3001\u7269\u4ef6\u548c\u6982\u5ff5\uff0c\u5e76\u628a\u5b83\u4eec\u5173\u8054\u5230\u7ae0\u8282\u4e0e\u5f7c\u6b64\u4e4b\u95f4\u3002',
  view: '\u4e16\u754c\u89c2\u8d44\u6599\u89c6\u56fe',
  cards: '\u8d44\u6599\u5361\u7247',
  relations: '\u5173\u8054\u89c6\u56fe',
  empty: '\u8fd8\u6ca1\u6709\u4e16\u754c\u89c2\u8d44\u6599\u3002\u5148\u6dfb\u52a0\u4e00\u4e2a\u4f1a\u53cd\u590d\u51fa\u73b0\u7684\u5730\u70b9\u3001\u7ec4\u7ec7\u6216\u8bbe\u5b9a\u3002',
  relationHeading: '\u8d44\u6599\u5173\u8054',
  outgoingCount: '\u6761\u51fa\u5411\u5173\u8054',
  outgoing: '\u6307\u5411',
  incoming: '\u88ab\u6307\u5411',
  noOutgoing: '\u6682\u65e0\u51fa\u5411\u5173\u8054',
  noIncoming: '\u6682\u65e0\u53cd\u5411\u5173\u8054',
  chapters: '\u4e2a\u7ae0\u8282\u5173\u8054',
  edit: '\u7f16\u8f91',
  add: '\u65b0\u5efa\u4e16\u754c\u89c2\u8d44\u6599',
  related: '\u5173\u8054\uff1a',
  editAsset: '\u7f16\u8f91\u4e16\u754c\u89c2\u8d44\u6599',
  deleteAsset: '\u5220\u9664\u4e16\u754c\u89c2\u8d44\u6599',
}

export function WorldAssetsView({ assets, chapters, busy, onCreate, onEdit, onDelete, onOpenChapter }: { assets: WorldAsset[]; chapters: Chapter[]; busy: boolean; onCreate: () => void; onEdit: (asset: WorldAsset) => void; onDelete: (asset: WorldAsset) => void; onOpenChapter: (chapterID: number) => void }) {
  const [view, setView] = useState<'cards' | 'relations'>('cards')
  const chapterNames = new Map(chapters.map((chapter) => [chapter.id, `\u7b2c${chapter.number}\u7ae0 \u00b7 ${chapter.title}`]))
  const assetNames = new Map(assets.map((asset) => [asset.id, asset.name]))
  const assetByID = new Map(assets.map((asset) => [asset.id, asset]))
  const incoming = useMemo(() => {
    const result = new Map<number, number[]>()
    for (const asset of assets) {
      for (const targetID of asset.related_asset_ids ?? []) result.set(targetID, [...(result.get(targetID) ?? []), asset.id])
    }
    return result
  }, [assets])
  const relationCount = assets.reduce((count, asset) => count + (asset.related_asset_ids ?? []).filter((id) => assetNames.has(id)).length, 0)

  return <div className="world-assets-view">
    <div className="collection-intro">
      <div className="collection-icon"><Globe2 size={16} /></div>
      <div><strong>{text.title}</strong><p>{text.intro}</p></div>
      <div className="world-asset-view-toggle" role="group" aria-label={text.view}>
        <button className={view === 'cards' ? 'active' : ''} onClick={() => setView('cards')} aria-pressed={view === 'cards'} title={text.cards}><Rows3 size={14} /></button>
        <button className={view === 'relations' ? 'active' : ''} onClick={() => setView('relations')} aria-pressed={view === 'relations'} title={text.relations}><Network size={14} /></button>
      </div>
    </div>

    {assets.length === 0 && <div className="collection-empty">{text.empty}</div>}

    {view === 'cards' && <div className="world-assets-grid">{assets.map((asset) => <article className="world-asset-card" key={asset.id}>
      <div className="world-asset-heading">
        <div><span className="world-asset-kind">{kindLabels[asset.kind] ?? asset.kind}</span><strong>{asset.name}</strong></div>
        <div className="collection-actions">
          <button className="subtle-icon" onClick={() => onEdit(asset)} disabled={busy} aria-label={`${text.editAsset}: ${asset.name}`}><Pencil size={13} /></button>
          <button className="subtle-icon danger" onClick={() => onDelete(asset)} disabled={busy} aria-label={`${text.deleteAsset}: ${asset.name}`}><Trash2 size={13} /></button>
        </div>
      </div>
      {asset.description && <p>{asset.description}</p>}
      {asset.details && <div className="world-asset-details">{asset.details}</div>}
      {(asset.chapter_ids?.length || asset.related_asset_ids?.length) ? <div className="world-asset-links">
        {asset.chapter_ids?.map((id) => {
          const name = chapterNames.get(id)
          return name ? <button type="button" key={`chapter-${id}`} onClick={() => onOpenChapter(id)} disabled={busy} title={name}>{name}</button> : null
        })}
        {asset.related_asset_ids?.map((id) => {
          const related = assetByID.get(id)
          return related ? <button type="button" key={`asset-${id}`} onClick={() => onEdit(related)} disabled={busy} title={`${text.editAsset}: ${related.name}`}>{text.related}{related.name}</button> : null
        })}
      </div> : null}
    </article>)}</div>}

    {view === 'relations' && assets.length > 0 && <section className="world-asset-relations" aria-label={text.relations}>
      <div className="world-asset-relations-heading"><strong>{text.relationHeading}</strong><span>{relationCount} {text.outgoingCount}</span></div>
      {assets.map((asset) => {
        const outgoing = (asset.related_asset_ids ?? []).map((id) => ({ id, name: assetNames.get(id) })).filter((item): item is { id: number; name: string } => Boolean(item.name))
        const linkedFrom = (incoming.get(asset.id) ?? []).map((id) => ({ id, name: assetNames.get(id) })).filter((item): item is { id: number; name: string } => Boolean(item.name))
        return <article className="world-asset-relation-row" key={asset.id}>
          <div className="world-asset-relation-node"><span className="world-asset-kind">{kindLabels[asset.kind] ?? asset.kind}</span><strong>{asset.name}</strong><small>{asset.chapter_ids?.length ?? 0} {text.chapters}</small></div>
          <ArrowRight size={15} className="world-asset-relation-arrow" />
          <div className="world-asset-relation-targets">
            <div><span className="world-asset-relation-label">{text.outgoing}</span>{outgoing.length > 0 ? outgoing.map((item) => <button type="button" className="world-asset-relation-chip" key={`out-${item.id}`} onClick={() => { const target = assetByID.get(item.id); if (target) onEdit(target) }} disabled={busy}>{item.name}</button>) : <span className="world-asset-relation-empty">{text.noOutgoing}</span>}</div>
            <div><span className="world-asset-relation-label">{text.incoming}</span>{linkedFrom.length > 0 ? linkedFrom.map((item) => <button type="button" className="world-asset-relation-chip muted" key={`in-${item.id}`} onClick={() => { const target = assetByID.get(item.id); if (target) onEdit(target) }} disabled={busy}>{item.name}</button>) : <span className="world-asset-relation-empty">{text.noIncoming}</span>}</div>
          </div>
          <button className="toolbar-button world-asset-relation-edit" onClick={() => onEdit(asset)} disabled={busy}><Pencil size={12} /> {text.edit}</button>
        </article>
      })}
    </section>}

    <button className="dashed-add" onClick={onCreate} disabled={busy}><Plus size={14} /> {text.add}</button>
  </div>
}
