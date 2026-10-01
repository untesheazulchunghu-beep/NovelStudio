<script setup lang="ts">
import type { Member } from '../api'

const memberEmail = defineModel<string>('memberEmail', { required: true })
const memberRole = defineModel<string>('memberRole', { required: true })

defineProps<{
  members: Member[]
  currentUserId: string
  workspaceRole: string
  roleLabel: string
}>()

const emit = defineEmits<{
  add: []
  changeRole: [member: Member, event: Event]
  remove: [member: Member]
}>()
</script>

<template>
  <section class="page team-page">
    <div class="page-heading">
      <div>
        <div class="eyebrow">WORKSPACE ACCESS</div>
        <h1>一起把故事写完。</h1>
        <p>管理工作区成员和项目权限。角色在工作区范围内生效。</p>
      </div>
      <span class="role-chip large-role">{{ roleLabel }}</span>
    </div>
    <div class="team-layout">
      <div class="team-main">
        <div class="section-heading">
          <div>
            <span class="eyebrow">MEMBERS</span>
            <h2>
              工作区成员 <span class="count-pill">{{ members.length }}</span>
            </h2>
          </div>
        </div>
        <div class="member-list">
          <div v-for="member in members" :key="member.id" class="member-row">
            <div class="avatar member-avatar">{{ member.email.slice(0, 1).toUpperCase() }}</div>
            <div class="member-info">
              <strong>{{ member.email }}</strong>
              <small>{{ member.id === currentUserId ? '这是你' : '已加入工作区' }}</small>
            </div>
            <span v-if="member.role === 'owner'" class="owner-pill">所有者</span>
            <template v-else>
              <select
                :value="member.role"
                :disabled="workspaceRole !== 'owner'"
                @change="emit('changeRole', member, $event)"
              >
                <option value="editor">编辑</option>
                <option value="reviewer">审阅者</option>
                <option value="viewer">只读</option>
              </select>
              <button
                v-if="workspaceRole === 'owner'"
                class="icon-button remove-member"
                title="移除成员"
                @click="emit('remove', member)"
              >
                ×
              </button>
            </template>
          </div>
          <div v-if="!members.length" class="empty-row">尚无成员</div>
        </div>
        <div v-if="workspaceRole === 'owner'" class="invite-card">
          <div>
            <span class="eyebrow">INVITE A COLLABORATOR</span>
            <h3>添加协作者</h3>
            <p>成员需要先注册账户，再通过邮箱加入当前工作区。</p>
          </div>
          <div class="invite-form">
            <input v-model="memberEmail" type="email" placeholder="输入成员邮箱" />
            <select v-model="memberRole">
              <option value="editor">编辑 · 可修改与生成</option>
              <option value="reviewer">审阅 · 可审核提案</option>
              <option value="viewer">只读 · 可查看项目</option>
            </select>
            <button class="button primary" :disabled="!memberEmail.trim()" @click="emit('add')">添加成员</button>
          </div>
        </div>
      </div>
      <aside class="role-guide">
        <span class="eyebrow">ROLE GUIDE</span>
        <h2>清晰的协作边界</h2>
        <div class="role-guide-item">
          <span class="role-icon owner-icon">O</span>
          <div>
            <strong>所有者</strong>
            <p>管理成员、设置审批方式、编辑和审核所有内容。</p>
          </div>
        </div>
        <div class="role-guide-item">
          <span class="role-icon editor-icon">E</span>
          <div>
            <strong>编辑</strong>
            <p>维护设定与章节，启动 Agent 和批量创作任务。</p>
          </div>
        </div>
        <div class="role-guide-item">
          <span class="role-icon reviewer-icon">R</span>
          <div>
            <strong>审阅者</strong>
            <p>查看项目内容，并批准或拒绝生成提案。</p>
          </div>
        </div>
        <div class="role-guide-item">
          <span class="role-icon viewer-icon">V</span>
          <div>
            <strong>只读</strong>
            <p>查看作品、知识图谱和工作流执行记录。</p>
          </div>
        </div>
      </aside>
    </div>
  </section>
</template>
