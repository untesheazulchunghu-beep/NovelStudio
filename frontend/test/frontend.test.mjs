import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'
import ts from 'typescript'

async function loadTypeScriptModule(path) {
  const source = await readFile(new URL(path, import.meta.url), 'utf8')
  const output = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.ESNext,
      target: ts.ScriptTarget.ES2022,
    },
  }).outputText
  return import(`data:text/javascript;base64,${Buffer.from(output).toString('base64')}`)
}

test('paragraph diff aligns unchanged paragraphs around edits', async () => {
  const { diffParagraphs } = await loadTypeScriptModule('../src/proposal-diff.ts')
  assert.deepEqual(
    diffParagraphs('开场\n\n被删段落\n\n共同结尾', '开场\n\n共同结尾\n\n新段落'),
    [
      { kind: 'same', text: '开场' },
      { kind: 'removed', text: '被删段落' },
      { kind: 'same', text: '共同结尾' },
      { kind: 'added', text: '新段落' },
    ],
  )
})

test('paragraph diff handles empty and large chapters safely', async () => {
  const { diffParagraphs } = await loadTypeScriptModule('../src/proposal-diff.ts')
  assert.deepEqual(diffParagraphs('', '新增段落'), [{ kind: 'added', text: '新增段落' }])

  const current = Array.from({ length: 201 }, (_, index) => `旧${index}`).join('\n\n')
  const proposed = Array.from({ length: 201 }, (_, index) => `新${index}`).join('\n\n')
  const result = diffParagraphs(current, proposed)
  assert.equal(result.length, 402)
  assert.equal(result[0].kind, 'removed')
  assert.equal(result.at(-1).kind, 'added')
})

test('chapter saves send both revision tokens for conflict detection', async () => {
  const originalFetch = globalThis.fetch
  const requests = []
  globalThis.fetch = async (url, init) => {
    requests.push({ url: String(url), init })
    return new Response(JSON.stringify({ ok: true }), { status: 200, headers: { 'Content-Type': 'application/json' } })
  }
  try {
    const { api } = await loadTypeScriptModule('../src/api.ts')
    await api.saveChapter({
      id: 'chapter-1',
      novelId: 'novel-1',
      number: 1,
      title: '第一章',
      body: '正文',
      revision: 3,
      updatedAt: '2026-01-02T03:04:05Z',
      status: 'draft',
      goal: '目标',
      keyEvents: '',
      suspenseHook: '',
    })
    assert.equal(requests.length, 1)
    assert.equal(requests[0].url, '/api/v1/novels/novel-1/chapters/chapter-1')
    assert.equal(requests[0].init.method, 'PUT')
    assert.deepEqual(JSON.parse(requests[0].init.body), {
      expectedRevision: 3,
      expectedUpdatedAt: '2026-01-02T03:04:05Z',
      title: '第一章',
      body: '正文',
      goal: '目标',
      keyEvents: '',
      suspenseHook: '',
    })
  } finally {
    globalThis.fetch = originalFetch
  }
})

test('novel search encodes the query and uses the scoped endpoint', async () => {
  const originalFetch = globalThis.fetch
  let request
  globalThis.fetch = async (url, init) => {
    request = { url: String(url), init }
    return new Response(JSON.stringify({ query: '港口 信', chapters: [] }), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    })
  }
  try {
    const { api } = await loadTypeScriptModule('../src/api.ts')
    await api.searchNovel('novel-1', '港口 信')
    assert.equal(request.url, '/api/v1/novels/novel-1/search?q=%E6%B8%AF%E5%8F%A3%20%E4%BF%A1&limit=20')
    assert.equal(request.init.credentials, 'same-origin')
  } finally {
    globalThis.fetch = originalFetch
  }
})

test('stale proposal regeneration targets the scoped endpoint', async () => {
  const originalFetch = globalThis.fetch
  let request
  globalThis.fetch = async (url, init) => {
    request = { url: String(url), init }
    return new Response(JSON.stringify({ id: 'task-2' }), { status: 202, headers: { 'Content-Type': 'application/json' } })
  }
  try {
    const { api } = await loadTypeScriptModule('../src/api.ts')
    await api.regenerateStaleProposal('novel-1', 'proposal-4')
    assert.equal(request.url, '/api/v1/novels/novel-1/proposals/proposal-4/regenerate')
    assert.equal(request.init.method, 'POST')
    assert.deepEqual(JSON.parse(request.init.body), {})
  } finally {
    globalThis.fetch = originalFetch
  }
})

test('selecting a chapter restores the cached draft for that chapter id and revision', async () => {
  const { draftStorageKey, restoreChapterDraft } = await loadTypeScriptModule('../src/draft.ts')
  const server = { title: '第一章', body: '服务器正文', goal: '原目标', keyEvents: '原事件', suspenseHook: '原钩子' }
  const cached = { title: '灯塔', body: '潮水拍打石阶。', goal: '抵达雾港', keyEvents: '发现来信', suspenseHook: '灯灭了' }
  const key = draftStorageKey('chapter-9', 4)
  assert.equal(key, 'novelstudio:draft:chapter-9:4')
  assert.notEqual(draftStorageKey('chapter-9', 5), key)
  assert.notEqual(draftStorageKey('chapter-8', 4), key)
  const restored = restoreChapterDraft(server, JSON.stringify(cached))
  assert.equal(restored.restored, true)
  assert.deepEqual(restored.draft, cached)
  const same = restoreChapterDraft(server, JSON.stringify(server))
  assert.equal(same.restored, false)
  assert.deepEqual(same.draft, server)
})

test('a failed save that is not a revision conflict keeps the local draft', async () => {
  const { planFailedChapterSave } = await loadTypeScriptModule('../src/draft.ts')
  const local = { title: '草稿', body: '还没写完的句子', goal: '目标', keyEvents: '事件', suspenseHook: '钩子' }
  const plan = planFailedChapterSave(local, '网络连接失败', null, 3)
  assert.equal(plan.retainLocalDraft, true)
  assert.equal(plan.overwriteServer, false)
  assert.equal(plan.recordConflict, false)
  assert.equal(plan.conflict, undefined)
  assert.deepEqual(plan.local, local)
})

test('invalid chapter text and a failed revision insert keep the local draft without a conflict', async () => {
  const originalFetch = globalThis.fetch
  const { isStaleChapterSave, planFailedChapterSave } = await loadTypeScriptModule('../src/draft.ts')
  const { api } = await loadTypeScriptModule('../src/api.ts')
  const local = { title: '', body: '本地正文', goal: '过长的蓝图', keyEvents: '事件', suspenseHook: '钩子' }
  const remote = { title: '服务器标题', body: '服务器正文', goal: '服务器目标', keyEvents: '', suspenseHook: '', revision: 4 }
  const chapter = {
    id: 'chapter-1',
    novelId: 'novel-1',
    number: 1,
    title: local.title,
    body: local.body,
    revision: 3,
    updatedAt: '2026-01-02T03:04:05Z',
    status: 'draft',
    goal: local.goal,
    keyEvents: local.keyEvents,
    suspenseHook: local.suspenseHook,
  }
  const failures = [
    { status: 400, code: 'INVALID_CHAPTER', message: '章节标题、版本号或蓝图字段无效' },
    { status: 500, code: 'DATABASE_ERROR', message: '记录章节版本失败' },
  ]
  globalThis.fetch = async () => {
    const failure = failures.shift()
    return new Response(JSON.stringify({ error: failure }), {
      status: failure.status,
      headers: { 'Content-Type': 'application/json' },
    })
  }
  try {
    for (const failure of [
      { status: 400, code: 'INVALID_CHAPTER', message: '章节标题、版本号或蓝图字段无效' },
      { status: 500, code: 'DATABASE_ERROR', message: '记录章节版本失败' },
    ]) {
      let caught
      try {
        await api.saveChapter(chapter)
      } catch (error) {
        caught = error
      }
      assert.equal(caught.code, failure.code)
      assert.equal(caught.message, failure.message)
      assert.equal(isStaleChapterSave(caught.code, caught.message), false)
      const plan = planFailedChapterSave(local, caught.message, remote, chapter.revision, caught.code)
      assert.equal(plan.retainLocalDraft, true)
      assert.equal(plan.overwriteServer, false)
      assert.equal(plan.recordConflict, false)
      assert.equal(plan.conflict, undefined)
      assert.deepEqual(plan.local, local)
    }
    assert.equal(isStaleChapterSave('STALE_REVISION', '章节标题、版本号或蓝图字段无效'), true)
    for (const message of [
      '章节已被其他人更新，请刷新后再保存',
      '章节蓝图或正文已被其他人更新，请比较后再保存',
      '章节已被其他人更新，请比较后再保存',
    ]) {
      const plan = planFailedChapterSave(local, message, remote, chapter.revision)
      assert.equal(plan.recordConflict, true)
      assert.equal(plan.conflict.baseRevision, chapter.revision)
    }
  } finally {
    globalThis.fetch = originalFetch
  }
})

test('a revision conflict keeps both drafts and does not overwrite unless confirmed', async () => {
  const { planFailedChapterSave, planConflictResolution } = await loadTypeScriptModule('../src/draft.ts')
  const local = { title: '本地标题', body: '本地正文', goal: '本地目标', keyEvents: '本地事件', suspenseHook: '本地钩子' }
  const remote = { title: '服务器标题', body: '服务器正文', goal: '服务器目标', keyEvents: '新事件', suspenseHook: '新钩子', revision: 8 }
  const failed = planFailedChapterSave(local, '章节已被其他人更新，请刷新后再保存', remote, 7)
  assert.equal(failed.retainLocalDraft, true)
  assert.equal(failed.overwriteServer, false)
  assert.equal(failed.recordConflict, true)
  assert.deepEqual(failed.conflict, { remote, baseRevision: 7 })
  const held = planConflictResolution(local, remote, false)
  assert.equal(held.overwriteServer, false)
  assert.deepEqual(held.local, local)
  assert.deepEqual(held.remote, remote)
  const overwrite = planConflictResolution(local, remote, true)
  assert.equal(overwrite.overwriteServer, true)
  assert.deepEqual(overwrite.local, local)
})

test('manuscript character count ignores whitespace', async () => {
  const { countManuscriptCharacters } = await loadTypeScriptModule('../src/word-count.ts')
  assert.equal(countManuscriptCharacters('雾 港\n来信\t'), 4)
  assert.equal(countManuscriptCharacters('  \n\t'), 0)
  assert.equal(countManuscriptCharacters('你好世界'), 4)
})

test('workflow connections reject self-links and duplicate edges', async () => {
  const { acceptWorkflowEdge, workflowEdgePath } = await loadTypeScriptModule('../src/workflow-connection.ts')
  const edges = [{ source: 'context', target: 'write' }]
  assert.deepEqual(acceptWorkflowEdge(edges, 'write', 'write'), { ok: false, reason: 'self' })
  assert.deepEqual(acceptWorkflowEdge(edges, 'context', 'write'), { ok: false, reason: 'duplicate' })
  assert.deepEqual(acceptWorkflowEdge(edges, '', 'write'), { ok: false, reason: 'missing' })
  assert.deepEqual(acceptWorkflowEdge(edges, 'write', 'output'), { ok: true })
  assert.equal(workflowEdgePath(undefined, { x: 10, y: 20 }), '')
  assert.equal(workflowEdgePath({ x: 10, y: 20 }, undefined), '')
  const source = { x: 36, y: 70 }
  const target = { x: 318, y: 70 }
  assert.equal(workflowEdgePath(source, target), 'M 226 132 C 282 132, 274 132, 330 132')
})

test('model settings use authenticated GET, PUT, and connection-test requests', async () => {
  const originalFetch = globalThis.fetch
  const requests = []
  globalThis.fetch = async (url, init) => {
    requests.push({ url: String(url), init })
    const body = requests.at(-1).url.endsWith('/test')
      ? { ok: true, message: '连接成功' }
      : { baseUrl: 'https://api.example.test/v1', model: 'novel-model', apiKeyConfigured: true }
    return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
  }
  try {
    const { api } = await loadTypeScriptModule('../src/api.ts')
    await api.modelSettings()
    await api.updateModelSettings({
      baseUrl: 'https://api.example.test/v1',
      model: 'novel-model',
      temperature: 0.6,
      maxTokens: 1024,
      timeoutSec: 120,
      apiKey: 'local-secret',
      clearApiKey: false,
    })
    await api.testModelSettings({
      baseUrl: 'https://api.example.test/v1',
      model: 'novel-model',
      temperature: 0.6,
      maxTokens: 1024,
      timeoutSec: 120,
      apiKey: 'local-secret',
      clearApiKey: false,
    })

    assert.deepEqual(requests.map(({ url, init }) => [url, init.method ?? 'GET']), [
      ['/api/v1/settings/llm', 'GET'],
      ['/api/v1/settings/llm', 'PUT'],
      ['/api/v1/settings/llm/test', 'POST'],
    ])
    assert.deepEqual(JSON.parse(requests[1].init.body), JSON.parse(requests[2].init.body))
    assert.equal(requests[1].init.credentials, 'same-origin')
    assert.equal(requests[2].init.credentials, 'same-origin')
  } finally {
    globalThis.fetch = originalFetch
  }
})

test('chapter memory refresh uses the chapter-scoped POST endpoint', async () => {
  const originalFetch = globalThis.fetch
  let request
  globalThis.fetch = async (url, init) => {
    request = { url: String(url), init }
    return new Response(JSON.stringify({ memoryStatus: 'current' }), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    })
  }
  try {
    const { api } = await loadTypeScriptModule('../src/api.ts')
    const response = await api.refreshChapterMemory('novel-1', 'chapter-2')
    assert.equal(request.url, '/api/v1/novels/novel-1/chapters/chapter-2/memory/refresh')
    assert.equal(request.init.method, 'POST')
    assert.deepEqual(JSON.parse(request.init.body), {})
    assert.equal(response.memoryStatus, 'current')
  } finally {
    globalThis.fetch = originalFetch
  }
})
