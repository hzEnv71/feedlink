<template>
  <div class="discover-page">
    <div class="card">
      <h2 class="page-title">🔍 发现用户</h2>
      <el-input
        v-model="keyword"
        placeholder="搜索用户名或昵称"
        size="large"
        prefix-icon="Search"
        clearable
        @keyup.enter="searchUsers"
        @clear="clearSearch"
      >
        <template #append>
          <el-button :loading="searching" @click="searchUsers">搜索</el-button>
        </template>
      </el-input>
    </div>

    <!-- 搜索结果 -->
    <div v-if="searched" class="mt-20">
      <div v-if="users.length === 0" class="text-center">
        <el-empty description="未找到相关用户" />
      </div>

      <div v-else class="user-grid">
        <div v-for="user in users" :key="user.id" class="card user-card" @click="goToProfile(user.id)">
          <div class="user-card-header">
            <el-avatar :size="60">
              {{ user.nickname?.charAt(0) || 'U' }}
            </el-avatar>
            <div class="user-card-info">
              <div class="user-card-name">
                {{ user.nickname }}
                <el-tag v-if="user.is_big_v" size="small" type="warning" effect="plain">大V</el-tag>
              </div>
              <div class="user-card-username">@{{ user.username }}</div>
              <div class="user-card-stats">
                <span>{{ user.follower_count }} 粉丝</span>
                <span>·</span>
                <span>{{ user.follow_count }} 关注</span>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- 分页 -->
      <div v-if="total > pageSize" class="text-center mt-20">
        <el-pagination
          v-model:current-page="page"
          :page-size="pageSize"
          :total="total"
          layout="prev, pager, next"
          @current-change="searchUsers"
        />
      </div>
    </div>

    <!-- 推荐用户（未搜索时显示） -->
    <div v-else class="mt-20">
      <div class="card">
        <h3>💡 提示</h3>
        <p style="color: #666; margin-top: 8px;">
          搜索你感兴趣的用户，关注他们来获取最新动态。
          Feed流系统采用推拉混合策略：
        </p>
        <ul style="color: #888; margin-top: 8px; padding-left: 20px; line-height: 2;">
          <li><strong>普通用户</strong>发布动态时，系统会自动<el-tag size="small" type="success">推送</el-tag>到粉丝的收件箱</li>
          <li><strong>大V用户</strong>（粉丝超过阈值）发布动态时，粉丝会实时<el-tag size="small" type="primary">拉取</el-tag>合并展示</li>
          <li>这种混合策略兼顾了实时性和系统性能</li>
        </ul>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { userApi } from '../api'

const router = useRouter()

const keyword = ref('')
const users = ref([])
const searching = ref(false)
const searched = ref(false)
const page = ref(1)
const pageSize = 20
const total = ref(0)

async function searchUsers() {
  if (!keyword.value.trim()) return
  searching.value = true
  searched.value = true
  try {
    const res = await userApi.searchUsers(keyword.value, page.value, pageSize)
    users.value = res.data.list || []
    total.value = res.data.total
  } catch (e) {}
  finally {
    searching.value = false
  }
}

function clearSearch() {
  searched.value = false
  users.value = []
  page.value = 1
}

function goToProfile(userId) {
  router.push(`/profile/${userId}`)
}
</script>

<style scoped>
.page-title {
  margin-bottom: 16px;
  font-size: 20px;
}

.user-grid {
  display: grid;
  gap: 12px;
}

.user-card {
  cursor: pointer;
  transition: all 0.2s;
}

.user-card:hover {
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
  transform: translateY(-1px);
}

.user-card-header {
  display: flex;
  gap: 16px;
  align-items: center;
}

.user-card-name {
  font-size: 16px;
  font-weight: 600;
  display: flex;
  align-items: center;
  gap: 6px;
}

.user-card-username {
  color: #999;
  font-size: 13px;
  margin-top: 2px;
}

.user-card-stats {
  display: flex;
  gap: 8px;
  color: #666;
  font-size: 13px;
  margin-top: 6px;
}
</style>