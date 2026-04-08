<template>
  <div class="profile-page" v-if="user">
    <!-- 用户信息卡片 -->
    <div class="card profile-card">
      <div class="profile-header">
        <div class="avatar-edit-wrap">
          <el-avatar :size="80" :src="user.avatar || ''">
            {{ user.nickname?.charAt(0) || 'U' }}
          </el-avatar>
          <el-upload
            v-if="isMe"
            class="avatar-uploader"
            :show-file-list="false"
            :auto-upload="false"
            accept="image/*"
            :on-change="handleAvatarSelect"
          >
            <el-button size="small" :loading="avatarUploading">更换头像</el-button>
          </el-upload>
        </div>
        <div class="profile-info">
          <h2 class="profile-name">
            {{ user.nickname }}
            <el-tag v-if="user.is_big_v" size="small" type="warning" effect="plain">大V</el-tag>
          </h2>
          <p class="profile-username">@{{ user.username }}</p>
          <div class="profile-bio-wrap">
            <p class="profile-bio" v-if="user.bio">{{ user.bio }}</p>
            <p class="profile-bio profile-bio-empty" v-else>这个人很懒，还没有签名</p>
            <el-button v-if="isMe" text type="primary" @click="openBioDialog">编辑签名</el-button>
          </div>
        </div>
      </div>

      <div class="profile-stats">
        <div class="stat-item" @click="activeTab = 'feeds'">
          <span class="stat-value">{{ feedTotal }}</span>
          <span class="stat-label">动态</span>
        </div>
        <div class="stat-item" @click="showFollowers">
          <span class="stat-value">{{ user.follower_count }}</span>
          <span class="stat-label">粉丝</span>
        </div>
        <div class="stat-item" @click="showFollowing">
          <span class="stat-value">{{ user.follow_count }}</span>
          <span class="stat-label">关注</span>
        </div>
        <div class="stat-item" v-if="isMe" @click="showVisitors">
          <span class="stat-value">{{ visitorTotal }}</span>
          <span class="stat-label">访客</span>
        </div>
      </div>

      <div class="profile-action" v-if="!isMe">
        <el-button
          :type="user.is_followed ? 'default' : 'primary'"
          :loading="followLoading"
          @click="handleFollowToggle"
        >
          {{ user.is_followed ? '取消关注' : '关注' }}
        </el-button>
      </div>
    </div>

    <!-- 动态列表 -->
    <div class="mt-20">
      <div v-if="feedLoading && feeds.length === 0">
        <el-skeleton :rows="3" animated />
      </div>

      <div v-else-if="feeds.length === 0" class="text-center">
        <el-empty description="暂无动态" />
      </div>

      <div v-else>
        <FeedCard
          v-for="feed in feeds"
          :key="feed.id"
          :feed="feed"
          :can-delete-feed="isMe"
          @like="handleLike"
          @unlike="handleUnlike"
          @delete="handleDelete"
          @click-author="goToProfile"
          @repost-success="loadFeeds"
        />

        <div class="load-more text-center mt-20">
          <el-button v-if="feedHasMore" :loading="feedLoadingMore" text @click="loadMoreFeeds">
            加载更多
          </el-button>
          <el-text v-else-if="feeds.length > 0" type="info">没有更多了</el-text>
        </div>
      </div>
    </div>

    <!-- 粉丝/关注/访客列表弹窗 -->
    <el-dialog v-model="dialogVisible" :title="dialogTitle" width="500px">
      <div v-if="dialogUsers.length === 0" class="text-center">
        <el-empty :description="dialogTitle + '列表为空'" />
      </div>
      <div v-else class="user-list">
        <div v-for="u in dialogUsers" :key="u.id" class="user-list-item" @click="goToProfile(u.id); dialogVisible = false;">
          <el-avatar :size="40" :src="u.avatar || ''">{{ u.nickname?.charAt(0) || 'U' }}</el-avatar>
          <div class="user-list-info">
            <div class="user-list-name">
              {{ u.nickname }}
              <el-tag v-if="u.is_big_v" size="small" type="warning" effect="plain">大V</el-tag>
            </div>
            <div class="user-list-username">@{{ u.username }}</div>
            <div v-if="dialogTitle === '最近访客' && u.visited_at" class="user-list-visit-time">访问于 {{ formatVisitTime(u.visited_at) }}</div>
          </div>
        </div>
      </div>
    </el-dialog>

    <el-dialog v-model="bioDialogVisible" title="编辑个性签名" width="520px">
      <el-input
        v-model="bioInput"
        type="textarea"
        :rows="4"
        maxlength="500"
        show-word-limit
        placeholder="写点介绍自己的一句话吧..."
      />
      <template #footer>
        <el-button @click="bioDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="bioSaving" @click="saveBio">保存</el-button>
      </template>
    </el-dialog>
  </div>

  <div v-else class="text-center mt-20">
    <el-skeleton :rows="5" animated />
  </div>
</template>

<script setup>
import { ref, onMounted, watch, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useUserStore } from '../stores/user'
import { userApi, followApi, feedApi, uploadApi } from '../api'
import { ElMessage } from 'element-plus'
import FeedCard from '../components/FeedCard.vue'

const route = useRoute()
const router = useRouter()
const userStore = useUserStore()

const user = ref(null)
const feeds = ref([])
const feedLoading = ref(false)
const feedLoadingMore = ref(false)
const feedPage = ref(1)
const feedTotal = ref(0)
const feedHasMore = ref(false)
const followLoading = ref(false)
const avatarUploading = ref(false)
const bioDialogVisible = ref(false)
const bioInput = ref('')
const bioSaving = ref(false)

const dialogVisible = ref(false)
const dialogTitle = ref('')
const dialogUsers = ref([])
const visitorTotal = ref(0)

const isMe = computed(() => userStore.userInfo?.id === user.value?.id)

onMounted(() => {
  loadProfile()
})

watch(() => route.params.id, () => {
  loadProfile()
})

async function loadProfile() {
  const userId = route.params.id
  try {
    const res = await userApi.getUserProfile(userId)
    user.value = res.data

    if (userStore.userInfo?.id === user.value?.id) {
      try {
        const visitorRes = await userApi.getRecentVisitors(1, 1)
        visitorTotal.value = visitorRes.data.total || 0
      } catch (e) {
        visitorTotal.value = 0
      }
    } else {
      visitorTotal.value = 0
    }

    loadFeeds()
  } catch (e) {}
}

async function loadFeeds() {
  feedLoading.value = true
  feedPage.value = 1
  try {
    const res = await feedApi.getUserFeeds(user.value.id, 1, 20)
    feeds.value = res.data.list || []
    feedTotal.value = res.data.total
    feedHasMore.value = res.data.has_more
  } catch (e) {}
  finally {
    feedLoading.value = false
  }
}

async function loadMoreFeeds() {
  feedLoadingMore.value = true
  feedPage.value++
  try {
    const res = await feedApi.getUserFeeds(user.value.id, feedPage.value, 20)
    feeds.value.push(...(res.data.list || []))
    feedHasMore.value = res.data.has_more
  } catch (e) {
    feedPage.value--
  } finally {
    feedLoadingMore.value = false
  }
}

async function handleFollowToggle() {
  followLoading.value = true
  try {
    if (user.value.is_followed) {
      await followApi.unfollow(user.value.id)
      user.value.is_followed = false
      user.value.follower_count = Math.max(0, user.value.follower_count - 1)
      ElMessage.success('已取消关注')
    } else {
      await followApi.follow(user.value.id)
      user.value.is_followed = true
      user.value.follower_count++
      ElMessage.success('关注成功')
    }
  } catch (e) {}
  finally {
    followLoading.value = false
  }
}

async function showFollowers() {
  dialogTitle.value = '粉丝'
  try {
    const res = await followApi.getFollowers(user.value.id, 1, 50)
    dialogUsers.value = res.data.list || []
    dialogVisible.value = true
  } catch (e) {}
}

async function showFollowing() {
  dialogTitle.value = '关注'
  try {
    const res = await followApi.getFollowing(user.value.id, 1, 50)
    dialogUsers.value = res.data.list || []
    dialogVisible.value = true
  } catch (e) {}
}

async function showVisitors() {
  dialogTitle.value = '最近访客'
  try {
    const res = await userApi.getRecentVisitors(1, 50)
    visitorTotal.value = res.data.total || 0
    dialogUsers.value = (res.data.list || []).map(item => ({
      ...(item.visitor || {}),
      visited_at: item.visited_at,
    }))
    dialogVisible.value = true
  } catch (e) {}
}

async function handleLike(feedId) {
  try {
    await feedApi.like(feedId)
    const feed = feeds.value.find(f => f.id === feedId)
    if (feed) {
      feed.is_liked = true
      feed.like_count++
    }
  } catch (e) {}
}

async function handleUnlike(feedId) {
  try {
    await feedApi.unlike(feedId)
    const feed = feeds.value.find(f => f.id === feedId)
    if (feed) {
      feed.is_liked = false
      feed.like_count = Math.max(0, feed.like_count - 1)
    }
  } catch (e) {}
}

async function handleDelete(feedId) {
  try {
    await feedApi.deleteFeed(feedId)
    feeds.value = feeds.value.filter(f => f.id !== feedId)
    feedTotal.value = Math.max(0, feedTotal.value - 1)
    ElMessage.success('删除成功')
  } catch (e) {}
}

async function handleAvatarSelect(uploadFile) {
  if (!isMe.value) return

  const rawFile = uploadFile?.raw
  if (!rawFile) return

  avatarUploading.value = true
  try {
    const uploadRes = await uploadApi.uploadImage(rawFile)
    const avatarUrl = uploadRes.data?.url || ''
    const updateRes = await userApi.updateProfile({ avatar: avatarUrl })

    user.value = updateRes.data
    if (userStore.userInfo?.id === user.value.id) {
      userStore.userInfo = { ...userStore.userInfo, ...updateRes.data }
      sessionStorage.setItem('user', JSON.stringify(userStore.userInfo))
    }

    ElMessage.success('头像更新成功')
  } catch (e) {
    // handled by interceptor
  } finally {
    avatarUploading.value = false
  }
}

function openBioDialog() {
  bioInput.value = user.value?.bio || ''
  bioDialogVisible.value = true
}

async function saveBio() {
  bioSaving.value = true
  try {
    const res = await userApi.updateProfile({ bio: bioInput.value.trim() })
    user.value = res.data
    if (userStore.userInfo?.id === user.value.id) {
      userStore.userInfo = { ...userStore.userInfo, ...res.data }
      sessionStorage.setItem('user', JSON.stringify(userStore.userInfo))
    }
    bioDialogVisible.value = false
    ElMessage.success('签名更新成功')
  } catch (e) {
    // handled by interceptor
  } finally {
    bioSaving.value = false
  }
}

function formatVisitTime(timeStr) {
  if (!timeStr) return ''
  const date = new Date(timeStr)
  if (Number.isNaN(date.getTime())) return ''

  const now = new Date()
  const diff = Math.floor((now - date) / 1000)
  if (diff < 60) return '刚刚'
  if (diff < 3600) return `${Math.floor(diff / 60)}分钟前`
  if (diff < 86400) return `${Math.floor(diff / 3600)}小时前`
  return `${date.getMonth() + 1}月${date.getDate()}日 ${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}`
}

function goToProfile(userId) {
  router.push(`/profile/${userId}`)
}
</script>

<style scoped>
.profile-card {
  text-align: center;
}

.profile-header {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
}

.avatar-edit-wrap {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
}

.profile-name {
  font-size: 22px;
  margin: 0;
  display: flex;
  align-items: center;
  gap: 8px;
  justify-content: center;
}

.profile-username {
  color: #999;
  margin: 4px 0;
}

.profile-bio-wrap {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
}

.profile-bio {
  color: #666;
  font-size: 14px;
  margin-top: 8px;
}

.profile-bio-empty {
  color: #999;
}

.profile-stats {
  display: flex;
  justify-content: center;
  gap: 40px;
  margin: 20px 0;
}

.stat-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  cursor: pointer;
}

.stat-item:hover .stat-value {
  color: #667eea;
}

.stat-value {
  font-size: 20px;
  font-weight: bold;
  color: #333;
}

.stat-label {
  font-size: 13px;
  color: #999;
  margin-top: 4px;
}

.profile-action {
  margin-top: 16px;
}

.user-list {
  max-height: 400px;
  overflow-y: auto;
}

.user-list-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px;
  border-radius: 8px;
  cursor: pointer;
  transition: background 0.2s;
}

.user-list-item:hover {
  background: #f5f7fa;
}

.user-list-name {
  font-weight: 500;
  display: flex;
  align-items: center;
  gap: 6px;
}

.user-list-username {
  font-size: 12px;
  color: #999;
}

.user-list-visit-time {
  font-size: 12px;
  color: #b0b6c3;
  margin-top: 2px;
}
</style>