package amazonsession

import "github.com/redis/go-redis/v9"

var (
	allSessionCmd = redis.NewScript(`
		local keys = redis.call("KEYS", "*:cookies")
		local res = {}
		for _, key in ipairs(keys) do
			local countryCode = string.match(key, "(.-):cookies")
			local sessionIdsKey = countryCode .. ":session-ids"
			local sessionIds = redis.call("LRANGE", sessionIdsKey, 0, -1)
			for _, sessionId in ipairs(sessionIds) do
				local lastCheckedKey = sessionId .. ":last-checked"
				local usageCountKey = sessionId .. ":usage-count"
				local createdAtKey = sessionId .. ":created-at"
				table.insert(res, countryCode)
				table.insert(res, sessionId)
				table.insert(res, redis.call("HGET", key, sessionId))
				table.insert(res, redis.call("HGET", key, lastCheckedKey))
				table.insert(res, redis.call("HGET", key, usageCountKey))
				table.insert(res, redis.call("HGET", key, createdAtKey))
			end
		end
		return res
	`)
	// KEYS[1] -> key for id list (e.g. {<country>}:session-ids)
	// KEYS[2] -> key for id list (e.g. {<country>}:cookies)
	// ARGV[1] -> start offset
	// ARGV[2] -> stop offset
	listSessionCmd = redis.NewScript(`
		local ids = redis.call("LRange", KEYS[1], ARGV[1], ARGV[2])
		local data = {}
		for _, id in ipairs(ids) do
			local lastCheckedKey = id .. ":last-checked"
			local usageCountKey = id .. ":usage-count"
			local createdAtKey = sessionId .. ":created-at"
			table.insert(data, id)
			table.insert(data, redis.call("HGET", KEYS[2], id))
			table.insert(data, redis.call("HGET", KEYS[2], usageCountKey))
			table.insert(data, redis.call("HGET", KEYS[2], lastCheckedKey))
			table.insert(data, redis.call("HGET", KEYS[2], createdAtKey))
		end
		return data
	`)
	// KEYS[1] -> key for id list (e.g. {<country>}:cookies)
	// ARGV[1] -> session id key
	// ARGV[2] -> usageCount Key
	// ARGV[3] -> lastChecked Key
	getSessionCmd = redis.NewScript(`
		local cookies = redis.call("HGET", KEYS[1], ARGV[1])
		local usageCount = redis.call("HINCRBY", KEYS[1], ARGV[2], 1)
		local lastCheck = redis.call("HGET", KEYS[1], ARGV[3])
		local createdAt = redis.call("HGET", KEYS[1], ARGV[4])
		if not cookies then
			return redis.error_reply("NOT FOUND")
		end
		return {cookies, usageCount, lastCheck, createdAt}
	`)
	// ARGV[1] -> currentTime
	// ARGV[2] -> timeDiff
	// ARGV[3] -> usageCount
	cleanupSessionsCmd = redis.NewScript(`
		local keys = redis.call("KEYS", "*:cookies")
		for _, key in ipairs(keys) do
			local countryCode = string.match(key, "(.-):cookies")
			local sessionIdsKey = countryCode .. ":session-ids"
			local sessionIds = redis.call("LRANGE", sessionIdsKey, 0, -1)
			for _, sessionId in ipairs(sessionIds) do
				local lastCheckedKey = sessionId .. ":last-checked"
				local usageCountKey = sessionId .. ":usage-count"
				local lastChecked = redis.call("HGET", key, lastCheckedKey)
				local usageCount = redis.call("HGET", key, usageCountKey)
				if lastChecked then
					local lastCheckedTime = tonumber(lastChecked)
					local currentTime = tonumber(ARGV[1])
					local timeDiff = currentTime - lastCheckedTime
					if timeDiff >= tonumber(ARGV[2]) or (usageCount and tonumber(usageCount) >= tonumber(ARGV[3])) then
						redis.call("LREM", sessionIdsKey,0, sessionId)
						redis.call("HDEL",key, sessionId)
						redis.call("HDEL",key, lastCheckedKey)
						redis.call("HDEL",key, usageCountKey)
					end
				end
			end
		end
		return redis.status_reply("OK")
	`)
)
