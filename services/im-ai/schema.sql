-- yuim im-ai MySQL schema (v9)
-- 目标：兼容现有 chat_msg，并支持 Outbox 最终一致 + 消费幂等 + 单聊/群聊按会话增量同步 + 群成员展开

CREATE TABLE IF NOT EXISTS im_conv_seq (
  conv_id VARCHAR(128) NOT NULL PRIMARY KEY,
  seq BIGINT NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS chat_msg  (
  msg_id BIGINT(20) NOT NULL COMMENT '消息主键',
  sync_id BIGINT(20) NULL DEFAULT NULL COMMENT '同步id(会话内递增)',
  user_id BIGINT(20) NULL DEFAULT NULL COMMENT '发送人',
  receive_id BIGINT(20) NULL DEFAULT NULL COMMENT '接收人',
  group_id BIGINT(20) NULL DEFAULT NULL COMMENT '群id',
  talk_type CHAR(1) NULL DEFAULT NULL COMMENT '聊天类型 1=单聊 2=群聊',
  msg_type VARCHAR(20) NULL DEFAULT NULL COMMENT '消息类型',
  content LONGTEXT NULL COMMENT '消息内容(JSON)',
  create_time DATETIME NULL DEFAULT NULL COMMENT '创建时间',

  conv_id VARCHAR(128) NULL DEFAULT NULL COMMENT '会话ID(p2p:min:max 或 g:groupId)',
  client_msg_id VARCHAR(64) NULL DEFAULT NULL COMMENT '客户端幂等ID',
  PRIMARY KEY (msg_id) USING BTREE,

  KEY idx_conv_sync (conv_id, sync_id),
  KEY idx_uid_conv_sync (user_id, conv_id, sync_id),
  KEY idx_gid_conv_sync (group_id, conv_id, sync_id),
  UNIQUE KEY uk_conv_sync (conv_id, sync_id),
  UNIQUE KEY uk_user_client (user_id, client_msg_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci COMMENT='聊天消息' ROW_FORMAT=DYNAMIC;

CREATE TABLE IF NOT EXISTS im_outbox (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  event VARCHAR(32) NOT NULL,
  msg_id BIGINT NOT NULL,
  conv_id VARCHAR(128) NOT NULL,
  sync_id BIGINT NOT NULL,

  topic VARCHAR(128) NOT NULL,
  tag VARCHAR(64) NOT NULL DEFAULT '*',
  payload_json JSON NOT NULL,

  status TINYINT NOT NULL DEFAULT 0,
  retry_count INT NOT NULL DEFAULT 0,
  next_retry_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  last_error VARCHAR(255) NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

  UNIQUE KEY uk_event_msg (event, msg_id),
  KEY idx_status_next (status, next_retry_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS im_group (
  group_id BIGINT NOT NULL PRIMARY KEY,
  name VARCHAR(128) NOT NULL DEFAULT '',
  owner_uid BIGINT NOT NULL,
  status TINYINT NOT NULL DEFAULT 1,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS im_group_member (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  group_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  role TINYINT NOT NULL DEFAULT 0,
  status TINYINT NOT NULL DEFAULT 1,
  joined_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_group_user (group_id, user_id),
  KEY idx_group_status (group_id, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
