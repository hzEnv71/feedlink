<template>
  <div class="timeline-page">
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
          @click-feed="goToFeedDetail"
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
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { feedApi } from '../api'
import FeedCard from '../components/FeedCard.vue'

const router = useRouter()

const feeds = ref([])
const loading = ref(false)
const loadingMore = ref(false)
// 游标分页状态：cursor 从 0 开始，服务端返回 next_cursor。
const cursor = ref(0)
const pageSize = 20
const hasMore = ref(true)

onMounted(() => {
  loadTimeline()
})

// loadTimeline 首次加载：从 cursor=0 拉取第一批。
async function loadTimeline() {
  loading.value = true
  try {
    const res = await feedApi.getTimeline(0, pageSize)
    feeds.value = res.data.list || []
    hasMore.value = !!res.data.has_more
    cursor.value = Number(res.data.next_cursor || 0)
  } catch (e) {
    // handled by interceptor
  } finally {
    loading.value = false
  }
}

// loadMore 使用 next_cursor 继续拉取后续数据。
async function loadMore() {
  if (!hasMore.value) return
  loadingMore.value = true
  try {
    const res = await feedApi.getTimeline(cursor.value, pageSize)
    feeds.value.push(...(res.data.list || []))
    hasMore.value = !!res.data.has_more
    cursor.value = Number(res.data.next_cursor || cursor.value)
  } catch (e) {
    // handled by interceptor
  } finally {
    loadingMore.value = false
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

function goToFeedDetail(feedId) {
  if (!feedId) return
  router.push(`/feed/${feedId}`)
}
</script>

<style scoped>
.timeline-page {
  background: #f0f2f5;
  border-radius: 12px;
  padding: 14px 16px;
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
