-- yuim im-ai MySQL schema (v5: compatible with existing chat_msg + outbox)

CREATE TABLE IF NOT EXISTS im_conv_seq (
  conv_id VARCHAR(128) NOT NULL PRIMARY KEY,
  seq BIGINT NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Existing chat_msg (extended with conv_id/client_msg_id; keep original columns)
CREATE TABLE IF NOT EXISTS chat_msg  (
  msg_id BIGINT NOT NULL COMMENT '消息主键',
  sync_id BIGINT NULL DEFAULT NULL COMMENT '同步id (conv seq)',
  user_id BIGINT NULL DEFAULT NULL COMMENT '发送人',
  receive_id BIGINT NULL DEFAULT NULL COMMENT '接收人',
  group_id BIGINT NULL DEFAULT NULL COMMENT '群id',
  talk_type CHAR(1) NULL DEFAULT NULL COMMENT '聊天类型: 1单聊 2群聊',
  msg_type VARCHAR(20) NULL DEFAULT NULL COMMENT '消息类型',
  content LONGTEXT NULL COMMENT '消息内容(JSON)',
  create_time DATETIME NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',

  -- yuim extensions (recommended)
  conv_id VARCHAR(128) NULL DEFAULT NULL COMMENT '会话ID(p2p:uid1:uid2 or g:groupId)',
  client_msg_id VARCHAR(64) NOT NULL DEFAULT '' COMMENT '客户端幂等ID',

  PRIMARY KEY (msg_id) USING BTREE,
  UNIQUE KEY uk_conv_sync (conv_id, sync_id),
  UNIQUE KEY uk_from_client (user_id, client_msg_id),
  KEY idx_conv_sync (conv_id, sync_id),
  KEY idx_recv_conv_sync (receive_id, conv_id, sync_id),
  KEY idx_group_conv_sync (group_id, conv_id, sync_id)
) ENGINE=InnoDB CHARACTER SET=utf8mb4 COLLATE=utf8mb4_general_ci COMMENT='聊天消息' ROW_FORMAT=DYNAMIC;

CREATE TABLE IF NOT EXISTS im_outbox (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  topic VARCHAR(128) NOT NULL,
  tag VARCHAR(64) NOT NULL DEFAULT '*',
  payload_json JSON NOT NULL,
  status TINYINT NOT NULL DEFAULT 0, -- 0=pending, 1=sent
  retry_count INT NOT NULL DEFAULT 0,
  next_retry_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  last_error VARCHAR(255) NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  KEY idx_status_next (status, next_retry_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
