-- 题目04 业务一致性核查 SQL（由 check_consistency.py 逐条执行）
-- 每条核查：第一条 SELECT 返回违规行数（0 = PASS），其余 SELECT 为证据输出。

-- CHECK 1: 已上架/停售/暂停的剧集必须来自审核通过（有审核通过留痕）
SELECT COUNT(*) FROM episodes
WHERE status IN ('published','sales_stopped','paused')
AND id NOT IN (SELECT episode_id FROM op_logs WHERE action='审核通过');
-- 证据：各已上架剧集的状态与最近操作
SELECT id, seq, status FROM episodes WHERE status IN ('published','sales_stopped','paused') ORDER BY seq;

-- CHECK 2: 每集待审修订不超过一份
SELECT COUNT(*) FROM (
  SELECT episode_id, COUNT(*) c FROM revisions WHERE status='pending' GROUP BY episode_id HAVING c > 1
);
-- 证据：当前待审修订
SELECT episode_id, id, title FROM revisions WHERE status='pending';

-- CHECK 3: 每条购买记录必须对应一条权益（扣币与权益一一对应）
SELECT COUNT(*) FROM purchases p
WHERE (SELECT COUNT(*) FROM entitlements e WHERE e.purchase_id = p.id) <> 1;
-- 证据：购买与权益对应抽样
SELECT p.id, p.user_id, p.episode_id, p.price, e.status
FROM purchases p LEFT JOIN entitlements e ON e.purchase_id = p.id ORDER BY p.id DESC LIMIT 20;

-- CHECK 4: 同一用户同一集不重复扣币（重复购买必须有退款撤销旧权益）
SELECT COUNT(*) FROM (
  SELECT user_id, episode_id, COUNT(*) c FROM purchases GROUP BY user_id, episode_id HAVING c > 1
) t
WHERE (SELECT COUNT(*) FROM entitlements e
       WHERE e.user_id = t.user_id AND e.episode_id = t.episode_id AND e.status = 'refunded') = 0;
-- 证据：重复购买明细
SELECT user_id, episode_id, COUNT(*) AS purchase_count
FROM purchases GROUP BY user_id, episode_id HAVING COUNT(*) > 1;

-- CHECK 5: 任何用户余额不为负
SELECT COUNT(*) FROM users WHERE balance < 0;
-- 证据：全部用户余额
SELECT username, role, balance FROM users ORDER BY id;

-- CHECK 6: 暂停观看期间不得有新购买（购买时间晚于暂停操作留痕）
SELECT COUNT(*) FROM purchases p
JOIN episodes e ON e.id = p.episode_id
WHERE e.status = 'paused'
AND p.created_at > (SELECT MAX(l.created_at) FROM op_logs l WHERE l.episode_id = e.id AND l.action = '暂停观看');
-- 证据：暂停中的剧集与待处理退款
SELECT e.id, e.seq, e.status,
  (SELECT COUNT(*) FROM refunds r WHERE r.episode_id = e.id AND r.status = 'pending') AS pending_refunds
FROM episodes e WHERE e.status = 'paused';

-- CHECK 7: 已批准的退款必须恰好撤销一条权益，且金额退回
SELECT COUNT(*) FROM refunds r
WHERE r.status = 'approved'
AND (SELECT COUNT(*) FROM entitlements e WHERE e.id = r.entitlement_id AND e.status = 'refunded') <> 1;
-- 证据：已批准退款与对应权益状态
SELECT r.id, r.user_id, r.episode_id, r.amount, r.status, e.status AS ent_status
FROM refunds r LEFT JOIN entitlements e ON e.id = r.entitlement_id ORDER BY r.id;

-- CHECK 8: 同一用户同一集已批准退款不超过一次
SELECT COUNT(*) FROM (
  SELECT user_id, episode_id, COUNT(*) c FROM refunds WHERE status = 'approved'
  GROUP BY user_id, episode_id HAVING c > 1
);
-- 证据：全部退款单
SELECT id, user_id, episode_id, amount, status, reject_reason, processed_by FROM refunds ORDER BY id;

-- CHECK 9: 已上架剧集的价格一经发布不变（权益扣币价 = 发布价）
SELECT COUNT(*) FROM purchases p
JOIN episodes e ON e.id = p.episode_id
WHERE e.status IN ('published','sales_stopped','paused') AND p.price <> e.pub_price;
-- 证据：价格抽样
SELECT id, seq, pub_price FROM episodes WHERE status IN ('published','sales_stopped','paused');

-- CHECK 10: 免费集不应有任何购买记录
SELECT COUNT(*) FROM purchases p JOIN episodes e ON e.id = p.episode_id WHERE e.pub_is_free = 1;
