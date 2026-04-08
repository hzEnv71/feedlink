import request from './request'

// ==================== 认证相关 ====================
export const authApi = {
    register(data) {
        return request.post('/auth/register', data)
    },
    login(data) {
        return request.post('/auth/login', data)
    },
}

// ==================== 用户相关 ====================
export const userApi = {
    getCurrentUser() {
        return request.get('/users/me')
    },
    updateProfile(data) {
        return request.put('/users/me', data)
    },
    getUserProfile(id) {
        return request.get(`/users/${id}`)
    },
    searchUsers(keyword, page = 1, pageSize = 20) {
        return request.get('/users/search', { params: { keyword, page, page_size: pageSize } })
    },
    getRecentVisitors(page = 1, pageSize = 20) {
        return request.get('/users/me/visitors', { params: { page, page_size: pageSize } })
    },
}

// ==================== 上传相关 ====================
export const uploadApi = {
    uploadImage(file) {
        const formData = new FormData()
        formData.append('file', file)
        return request.post('/upload/image', formData, {
            headers: { 'Content-Type': 'multipart/form-data' },
        })
    },
    uploadVideo(file) {
        const formData = new FormData()
        formData.append('file', file)
        return request.post('/upload/video', formData, {
            headers: { 'Content-Type': 'multipart/form-data' },
        })
    },
}

// ==================== 关注相关 ====================
export const followApi = {
    follow(userId) {
        return request.post(`/follow/${userId}`)
    },
    unfollow(userId) {
        return request.delete(`/follow/${userId}`)
    },
    getFollowers(userId, page = 1, pageSize = 20) {
        return request.get(`/users/${userId}/followers`, { params: { page, page_size: pageSize } })
    },
    getFollowing(userId, page = 1, pageSize = 20) {
        return request.get(`/users/${userId}/following`, { params: { page, page_size: pageSize } })
    },
}

// ==================== Feed相关 ====================
export const feedApi = {
    publish(data) {
        return request.post('/feeds', data)
    },
    repost(data) {
        return request.post('/feeds/repost', data)
    },
    deleteFeed(id) {
        return request.delete(`/feeds/${id}`)
    },
    getFeed(id) {
        return request.get(`/feeds/${id}`)
    },
    getUserFeeds(userId, page = 1, pageSize = 20) {
        return request.get(`/users/${userId}/feeds`, { params: { page, page_size: pageSize } })
    },
    getTimeline(page = 1, pageSize = 20) {
        return request.get('/timeline', { params: { page, page_size: pageSize } })
    },
    like(feedId) {
        return request.post(`/feeds/${feedId}/like`)
    },
    unlike(feedId) {
        return request.delete(`/feeds/${feedId}/like`)
    },
    getLikers(feedId, page = 1, pageSize = 20) {
        return request.get(`/feeds/${feedId}/likes`, { params: { page, page_size: pageSize } })
    },
    getComments(feedId, page = 1, pageSize = 20) {
        return request.get(`/feeds/${feedId}/comments`, { params: { page, page_size: pageSize } })
    },
    addComment(feedId, content) {
        return request.post(`/feeds/${feedId}/comments`, { content })
    },
    deleteComment(feedId, commentId) {
        return request.delete(`/feeds/${feedId}/comments/${commentId}`)
    },
}

// ==================== 消息相关 ====================
export const messageApi = {
    sendMessage(toUserId, content) {
        return request.post('/messages', { to_user_id: toUserId, content })
    },
    getConversations(page = 1, pageSize = 20) {
        return request.get('/messages/conversations', { params: { page, page_size: pageSize } })
    },
    getMessages(targetUserId, page = 1, pageSize = 30) {
        return request.get(`/messages/${targetUserId}`, { params: { page, page_size: pageSize } })
    },
}
