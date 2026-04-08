<template>
  <div class="timeline-page">
    <section class="composer">
      <div class="composer-avatar">
        <el-avatar :size="44" :src="userStore.userInfo?.avatar || ''">{{ userStore.userInfo?.nickname?.charAt(0) || '我' }}</el-avatar>
      </div>

      <div class="composer-main">
        <el-input
          v-model="publishContent"
          type="textarea"
          :rows="3"
          maxlength="5000"
          resize="none"
          placeholder="这一刻的想法..."
        />

        <div v-if="mediaPreviews.length" class="composer-medias">
          <div v-for="(item, idx) in mediaPreviews" :key="item.url + idx" class="preview-item">
            <template v-if="item.kind === 'image'">
              <el-image :src="item.url" fit="cover" class="preview-image" />
            </template>
            <template v-else>
              <video class="preview-video" :src="item.url" controls preload="metadata" />
            </template>
            <button class="remove-btn" type="button" @click="removeMedia(idx)">×</button>
          </div>
        </div>

        <div class="composer-footer">
          <div class="composer-tools">
            <label class="upload-btn" for="moment-upload-input">
              <el-icon><Plus /></el-icon>
              <span>添加图片/视频</span>
            </label>
            <input
              id="moment-upload-input"
              class="hidden-input"
              type="file"
              accept="image/*,video/*"
              multiple
              @change="handleSelectMedia"
            />
            <span class="count">{{ publishContent.length }}/5000</span>
          </div>

          <el-button
            type="primary"
            class="publish-btn"
            :loading="publishing"
            :disabled="!canPublish"
            @click="handlePublish"
          >
            发表
          </el-button>
        </div>
      </div>
    </section>

    <section class="moments-list">
      <div v-if="loading && feeds.length === 0" class="loading-wrap">
        <el-skeleton :rows="4" animated />
        <el-skeleton :rows="4" animated class="mt-20" />
      </div>

      <div v-else-if="feeds.length === 0" class="empty-wrap">
        <el-empty description="朋友圈还没有动态">
          <el-button type="primary" @click="router.push('/discover')">去发现朋友</el-button>
        </el-empty>
      </div>

      <template v-else>
        <FeedCard
          v-for="feed in feeds"
          :key="feed.id"
          :feed="feed"
          :can-delete-feed="false"
          @like="handleLike"
          @unlike="handleUnlike"
          @delete="handleDelete"
          @click-author="goToProfile"
          @repost-success="loadTimeline"
        />

        <div class="load-more">
          <el-button v-if="hasMore" :loading="loadingMore" text @click="loadMore">加载更多</el-button>
          <span v-else>没有更多动态了</span>
        </div>
      </template>
    </section>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { useUserStore } from '../stores/user'
import { feedApi, uploadApi } from '../api'
import FeedCard from '../components/FeedCard.vue'

const router = useRouter()
const userStore = useUserStore()

const feeds = ref([])
const loading = ref(false)
const loadingMore = ref(false)
const publishing = ref(false)
const publishContent = ref('')
const mediaPreviews = ref([])
const uploadedImageUrls = ref([])
const uploadedVideoUrls = ref([])
const page = ref(1)
const pageSize = 20
const hasMore = ref(true)

const canPublish = computed(() => {
  return !!publishContent.value.trim() || uploadedImageUrls.value.length > 0 || uploadedVideoUrls.value.length > 0
})

onMounted(() => {
  loadTimeline()
})

async function loadTimeline() {
  loading.value = true
  try {
    const res = await feedApi.getTimeline(1, pageSize)
    feeds.value = res.data.list || []
    hasMore.value = res.data.has_more
    page.value = 1
  } catch (e) {
    // handled by interceptor
  } finally {
    loading.value = false
  }
}

async function loadMore() {
  if (!hasMore.value) return
  loadingMore.value = true
  try {
    const nextPage = page.value + 1
    const res = await feedApi.getTimeline(nextPage, pageSize)
    feeds.value.push(...(res.data.list || []))
    hasMore.value = res.data.has_more
    page.value = nextPage
  } catch (e) {
    // handled by interceptor
  } finally {
    loadingMore.value = false
  }
}

async function handleSelectMedia(event) {
  const files = Array.from(event.target.files || [])
  event.target.value = ''
  if (!files.length) return

  const mediaFiles = files.filter((file) => file.type?.startsWith('image/') || file.type?.startsWith('video/'))
  if (mediaFiles.length !== files.length) {
    ElMessage.warning('仅支持图片和视频文件')
  }
  if (!mediaFiles.length) return

  const allowedCount = Math.max(0, 9 - mediaPreviews.value.length)
  const picked = mediaFiles.slice(0, allowedCount)
  if (!picked.length) {
    ElMessage.warning('最多上传9个媒体文件')
    return
  }

  const previewItems = picked.map((file) => ({
    kind: file.type.startsWith('video/') ? 'video' : 'image',
    file,
    url: URL.createObjectURL(file),
    uploadedUrl: '',
  }))

  mediaPreviews.value.push(...previewItems)

  try {
    for (const item of previewItems) {
      if (item.kind === 'video') {
        const res = await uploadApi.uploadVideo(item.file)
        item.uploadedUrl = res.data.url
        uploadedVideoUrls.value.push(res.data.url)
      } else {
        const res = await uploadApi.uploadImage(item.file)
        item.uploadedUrl = res.data.url
        uploadedImageUrls.value.push(res.data.url)
      }
    }
  } catch (e) {
    // handled by interceptor
  }
}

function removeMedia(index) {
  const [removed] = mediaPreviews.value.splice(index, 1)
  if (!removed) return

  if (removed.kind === 'video') {
    const idx = uploadedVideoUrls.value.findIndex((u) => u === removed.uploadedUrl)
    if (idx >= 0) uploadedVideoUrls.value.splice(idx, 1)
  } else {
    const idx = uploadedImageUrls.value.findIndex((u) => u === removed.uploadedUrl)
    if (idx >= 0) uploadedImageUrls.value.splice(idx, 1)
  }

  if (removed.url?.startsWith('blob:')) URL.revokeObjectURL(removed.url)
}

async function handlePublish() {
  if (!canPublish.value) return
  publishing.value = true
  try {
    const payload = {
      content: publishContent.value.trim(),
      images: uploadedImageUrls.value.length ? JSON.stringify(uploadedImageUrls.value) : '',
      videos: uploadedVideoUrls.value.length ? JSON.stringify(uploadedVideoUrls.value) : '',
    }
    await feedApi.publish(payload)

    publishContent.value = ''
    mediaPreviews.value.forEach((item) => {
      if (item.url?.startsWith('blob:')) URL.revokeObjectURL(item.url)
    })
    mediaPreviews.value = []
    uploadedImageUrls.value = []
    uploadedVideoUrls.value = []

    ElMessage.success('发表成功')
    await loadTimeline()
  } catch (e) {
    // handled by interceptor
  } finally {
    publishing.value = false
  }
}

async function handleLike(feedId) {
  try {
    await feedApi.like(feedId)
    const feed = feeds.value.find((item) => item.id === feedId)
    if (feed) {
      feed.is_liked = true
      feed.like_count = (feed.like_count || 0) + 1
    }
  } catch (e) {
    // handled by interceptor
  }
}

async function handleUnlike(feedId) {
  try {
    await feedApi.unlike(feedId)
    const feed = feeds.value.find((item) => item.id === feedId)
    if (feed) {
      feed.is_liked = false
      feed.like_count = Math.max((feed.like_count || 0) - 1, 0)
    }
  } catch (e) {
    // handled by interceptor
  }
}

async function handleDelete(feedId) {
  try {
    await feedApi.deleteFeed(feedId)
    feeds.value = feeds.value.filter((item) => item.id !== feedId)
    ElMessage.success('删除成功')
  } catch (e) {
    // handled by interceptor
  }
}

function goToProfile(userId) {
  if (!userId) return
  router.push(`/profile/${userId}`)
}
</script>

<style scoped>
.timeline-page {
  background: #f0f2f5;
  border-radius: 12px;
  padding: 14px 16px;
}

.composer {
  display: flex;
  gap: 10px;
  background: #fff;
  border-radius: 8px;
  padding: 12px;
}

.composer-main {
  flex: 1;
}

.composer-medias {
  margin-top: 10px;
  display: grid;
  grid-template-columns: repeat(3, 88px);
  gap: 8px;
}

.preview-item {
  position: relative;
  width: 88px;
  height: 88px;
}

.preview-image,
.preview-video {
  width: 100%;
  height: 100%;
  border-radius: 6px;
  background: #eef1f6;
  object-fit: cover;
}

.remove-btn {
  position: absolute;
  top: -6px;
  right: -6px;
  width: 18px;
  height: 18px;
  border: 0;
  border-radius: 999px;
  background: #2f3540;
  color: #fff;
  cursor: pointer;
}

.composer-footer {
  margin-top: 8px;
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.composer-tools {
  display: flex;
  align-items: center;
  gap: 10px;
}

.upload-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: #576b95;
  font-size: 13px;
  cursor: pointer;
}

.hidden-input {
  display: none;
}

.count {
  color: #9ba3af;
  font-size: 12px;
}

.publish-btn {
  min-width: 72px;
}

.moments-list {
  margin-top: 10px;
  background: #fff;
  border-radius: 8px;
  padding: 0 12px;
}

.loading-wrap,
.empty-wrap {
  padding: 16px 0;
}

.load-more {
  text-align: center;
  color: #a3a9b4;
  font-size: 12px;
  padding: 14px 0;
}
</style>
