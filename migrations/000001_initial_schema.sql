-- MedStudy v1 初始结构，适用于 PostgreSQL 18+ / UTF8。
-- 在 Navicat 中选中现有 medstudy 数据库，以 UTF-8「运行 SQL 文件」。
-- 仅用于尚未初始化的数据库；没有 DROP、TRUNCATE、账号密码或演示业务数据。
-- 全部 DDL 在一个事务内；重复导入会报错，不会覆盖或清空现有数据。
-- 修改已导入的结构请新增后续 migration，不要反复编辑并导入本文件。

BEGIN;
SET LOCAL client_encoding = 'UTF8';
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '120s';
SET LOCAL search_path = pg_catalog, public;

DO $requirements$
BEGIN
    IF current_setting('server_version_num')::integer < 180000 THEN
        RAISE EXCEPTION '此脚本需要 PostgreSQL 18 或更新版本';
    END IF;
    IF current_setting('server_encoding') <> 'UTF8' THEN
        RAISE EXCEPTION '此脚本需要 UTF8 数据库';
    END IF;
END;
$requirements$;

-- 只协调并发导入，不替代未来 migration runner 的版本检查。
SELECT pg_advisory_xact_lock(71483501);

CREATE TABLE public.schema_migrations (
    version bigint PRIMARY KEY,
    name text NOT NULL,
    applied_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT schema_migrations_version_check CHECK (version > 0)
);
COMMENT ON TABLE public.schema_migrations IS 'SQL 结构版本；业务快照的 schemaVersion 与此版本独立';

-- 对齐 Go strings.TrimSpace 的 Unicode White_Space 字符集合。
CREATE FUNCTION public.medstudy_trim_text(value text)
RETURNS text
LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
SET search_path = pg_catalog
AS $function$
    SELECT btrim(value,
        U&'\0009\000A\000B\000C\000D\0020\0085\00A0\1680\2000\2001\2002\2003\2004\2005\2006\2007\2008\2009\200A\2028\2029\202F\205F\3000');
$function$;

-- 去重用规范文本：NFKC → 去首尾空白 → Unicode 小写 → 合并空白。
-- 使用 pg_unicode_fast 避免依赖部署机器的默认语言环境。
CREATE FUNCTION public.medstudy_normalize_text(value text)
RETURNS text
LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
SET search_path = pg_catalog, public
AS $function$
    SELECT regexp_replace(
        lower(public.medstudy_trim_text(normalize(value, NFKC)) COLLATE "pg_unicode_fast"),
        U&'[\0009\000A\000B\000C\000D\0020\0085\00A0\1680\2000-\200A\2028\2029\202F\205F\3000]+',
        ' ', 'g');
$function$;

-- 固定 UTF8→UTF8 编码后生成 SHA-256；包装用于生成列的 immutable 表达式。
CREATE FUNCTION public.medstudy_text_hash(value text)
RETURNS bytea
LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
SET search_path = pg_catalog
AS $function$
    SELECT sha256(convert_to(value, 'UTF8'));
$function$;

-- 长文本不直接进入 B-tree 唯一索引。长度前缀保证 front/back 的拼接没有歧义。
CREATE FUNCTION public.medstudy_flashcard_content_hash(front_text text, back_text text)
RETURNS bytea
LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
SET search_path = pg_catalog, public
AS $function$
    SELECT sha256(convert_to(
        octet_length(front_value)::text || ':' || front_value ||
        octet_length(back_value)::text || ':' || back_value,
        'UTF8'))
    FROM (
        SELECT public.medstudy_normalize_text(front_text) AS front_value,
               public.medstudy_normalize_text(back_text) AS back_value
    ) AS normalized;
$function$;

-- options 必须是「字符串键 → 字符串值」的 JSON 对象，SQL NULL 表示没有选项。
CREATE FUNCTION public.medstudy_valid_options(value jsonb)
RETURNS boolean
LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE
SET search_path = pg_catalog
AS $function$
    SELECT CASE WHEN jsonb_typeof(value) = 'object' THEN
        NOT EXISTS (SELECT 1 FROM jsonb_each(value) AS item WHERE jsonb_typeof(item.value) <> 'string')
    ELSE false END;
$function$;

CREATE FUNCTION public.medstudy_touch_updated_at()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = pg_catalog
AS $function$
BEGIN
    NEW.updated_at := CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$function$;

-- 1. 账号。密码哈希由 Go 生成，不在 SQL 中保存或计算明文密码。
CREATE TABLE public.users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    username text NOT NULL,
    username_key text COLLATE "C" GENERATED ALWAYS AS (public.medstudy_normalize_text(username)) STORED,
    password_hash text NOT NULL,
    timezone text NOT NULL DEFAULT 'Asia/Shanghai',
    revision bigint NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT users_username_check CHECK (
        username = public.medstudy_trim_text(username) AND char_length(username) BETWEEN 3 AND 32),
    CONSTRAINT users_username_key_check CHECK (char_length(username_key) > 0),
    CONSTRAINT users_username_key_unique UNIQUE (username_key),
    CONSTRAINT users_password_hash_check CHECK (password_hash LIKE '$argon2id$%'),
    CONSTRAINT users_timezone_check CHECK (char_length(timezone) BETWEEN 1 AND 255),
    CONSTRAINT users_revision_check CHECK (revision >= 0),
    CONSTRAINT users_created_at_check CHECK (created_at >= TIMESTAMPTZ '1970-01-01 00:00:00+00')
);
COMMENT ON TABLE public.users IS '用户；Go 校验完整 Argon2id 编码及 IANA 时区，写操作在事务内更新 revision';
COMMENT ON COLUMN public.users.username_key IS '数据库生成的规范用户名，查询登录和唯一性检查都使用同一规范化函数';
COMMENT ON COLUMN public.users.revision IS '每用户业务数据版本；由 service 在成功写入或恢复事务中递增';

-- 2. refresh 会话。已轮换的旧行不能立即删除，用于识别旧 token 重放。
CREATE TABLE public.sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    token_hash bytea NOT NULL,
    family_id uuid NOT NULL DEFAULT gen_random_uuid(),
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    revoked_at timestamptz,
    replaced_by_id uuid,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT sessions_user_id_id_unique UNIQUE (user_id, id),
    CONSTRAINT sessions_token_hash_unique UNIQUE (token_hash),
    CONSTRAINT sessions_token_hash_check CHECK (octet_length(token_hash) = 32),
    CONSTRAINT sessions_expiry_check CHECK (expires_at > created_at),
    CONSTRAINT sessions_replacement_check CHECK (replaced_by_id IS NULL OR (replaced_by_id <> id AND consumed_at IS NOT NULL)),
    CONSTRAINT sessions_replacement_fk FOREIGN KEY (user_id, replaced_by_id)
        REFERENCES public.sessions(user_id, id) ON DELETE SET NULL (replaced_by_id)
);
CREATE INDEX sessions_family_idx ON public.sessions (user_id, family_id);
CREATE INDEX sessions_expiry_idx ON public.sessions (expires_at);
CREATE INDEX sessions_replacement_idx ON public.sessions (user_id, replaced_by_id) WHERE replaced_by_id IS NOT NULL;
COMMENT ON TABLE public.sessions IS '仅存 SHA-256 token 哈希；轮换、同 family 校验及重放吊销由 auth service 在事务内完成';

-- 3. 个人 AI Key。nonce/ciphertext 必须由 Go 的 AES-GCM 生成。
CREATE TABLE public.ai_credentials (
    user_id uuid NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    provider text NOT NULL DEFAULT 'deepseek',
    ciphertext bytea NOT NULL,
    nonce bytea NOT NULL,
    key_version integer NOT NULL,
    last4 text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, provider),
    CONSTRAINT ai_credentials_provider_check CHECK (provider = 'deepseek'),
    CONSTRAINT ai_credentials_ciphertext_check CHECK (octet_length(ciphertext) >= 16),
    CONSTRAINT ai_credentials_nonce_check CHECK (octet_length(nonce) = 12),
    CONSTRAINT ai_credentials_key_version_check CHECK (key_version > 0),
    CONSTRAINT ai_credentials_last4_check CHECK (char_length(last4) = 4)
);
COMMENT ON TABLE public.ai_credentials IS '个人 Key 密文；加密主密钥不进入数据库，last4 仅用于状态展示';

-- 4. 资料夹。同名资料夹允许存在，契约没有要求名称唯一。
CREATE TABLE public.folders (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    name text NOT NULL,
    type text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT folders_user_id_id_unique UNIQUE (user_id, id),
    CONSTRAINT folders_name_check CHECK (
        name = public.medstudy_trim_text(name) AND char_length(name) BETWEEN 1 AND 100),
    CONSTRAINT folders_type_check CHECK (type IN ('material', 'exam')),
    CONSTRAINT folders_created_at_check CHECK (created_at >= TIMESTAMPTZ '1970-01-01 00:00:00+00')
);
CREATE INDEX folders_list_idx ON public.folders (user_id, type, created_at, id);
CREATE INDEX folders_all_list_idx ON public.folders (user_id, created_at, id);
COMMENT ON TABLE public.folders IS '资料夹；删除时级联删除 documents，不删除来源卡片、错题或快刷题';

-- 5. 文档。folder_id 可空以兼容旧快照；新建接口仍必须指定已有资料夹。
CREATE TABLE public.documents (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    folder_id uuid,
    name text NOT NULL,
    content text NOT NULL,
    character_count integer GENERATED ALWAYS AS (char_length(content)) STORED,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT documents_user_id_id_unique UNIQUE (user_id, id),
    CONSTRAINT documents_folder_fk FOREIGN KEY (user_id, folder_id)
        REFERENCES public.folders(user_id, id) ON DELETE CASCADE,
    CONSTRAINT documents_name_check CHECK (
        name = public.medstudy_trim_text(name) AND char_length(name) BETWEEN 1 AND 255),
    CONSTRAINT documents_content_check CHECK (char_length(content) >= 10),
    CONSTRAINT documents_created_at_check CHECK (created_at >= TIMESTAMPTZ '1970-01-01 00:00:00+00')
);
CREATE INDEX documents_folder_list_idx ON public.documents (user_id, folder_id, created_at DESC, id DESC);
CREATE INDEX documents_all_list_idx ON public.documents (user_id, created_at DESC, id DESC);
COMMENT ON TABLE public.documents IS '解析文本；5 MiB 等可配置限额由 HTTP/service 检查，不硬编码为不可调 SQL 约束';
COMMENT ON COLUMN public.documents.character_count IS 'Unicode 字符数，不是 UTF-8 字节数；无需应用重复写入';

-- 6. 卡片。origin_id 是契约中的字符串，不强制它引用持久资源。
CREATE TABLE public.flashcards (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    front text NOT NULL,
    back text NOT NULL,
    content_hash bytea GENERATED ALWAYS AS (public.medstudy_flashcard_content_hash(front, back)) STORED,
    origin_type text,
    origin_id text COLLATE "C",
    origin_hash bytea GENERATED ALWAYS AS (public.medstudy_text_hash(origin_id)) STORED,
    source_doc_id uuid,
    source_doc_name text,
    source_folder_id uuid,
    source_folder_name text,
    source_text text,
    source_doc_ref_id uuid,
    source_folder_ref_id uuid,
    next_review_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    interval_days integer NOT NULL DEFAULT 0,
    ease_factor double precision NOT NULL DEFAULT 2.5,
    repetitions integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT flashcards_user_id_id_unique UNIQUE (user_id, id),
    CONSTRAINT flashcards_front_check CHECK (char_length(public.medstudy_normalize_text(front)) > 0),
    CONSTRAINT flashcards_back_check CHECK (char_length(public.medstudy_normalize_text(back)) > 0),
    CONSTRAINT flashcards_origin_type_check CHECK (origin_type IS NULL OR origin_type IN ('quick-quiz', 'error', 'exam', 'reinforcement')),
    CONSTRAINT flashcards_origin_id_check CHECK (origin_id IS NULL OR (
        origin_id = public.medstudy_trim_text(origin_id) AND char_length(origin_id) > 0)),
    CONSTRAINT flashcards_doc_ref_check CHECK (source_doc_ref_id IS NULL OR (source_doc_id IS NOT NULL AND source_doc_ref_id = source_doc_id)),
    CONSTRAINT flashcards_folder_ref_check CHECK (source_folder_ref_id IS NULL OR (source_folder_id IS NOT NULL AND source_folder_ref_id = source_folder_id)),
    CONSTRAINT flashcards_doc_fk FOREIGN KEY (user_id, source_doc_ref_id)
        REFERENCES public.documents(user_id, id) ON DELETE SET NULL (source_doc_ref_id),
    CONSTRAINT flashcards_folder_fk FOREIGN KEY (user_id, source_folder_ref_id)
        REFERENCES public.folders(user_id, id) ON DELETE SET NULL (source_folder_ref_id),
    CONSTRAINT flashcards_interval_check CHECK (interval_days >= 0),
    CONSTRAINT flashcards_ease_factor_check CHECK (ease_factor >= 1.3 AND ease_factor < 'Infinity'::double precision),
    CONSTRAINT flashcards_repetitions_check CHECK (repetitions >= 0),
    CONSTRAINT flashcards_review_time_check CHECK (next_review_at >= TIMESTAMPTZ '1970-01-01 00:00:00+00'),
    CONSTRAINT flashcards_created_at_check CHECK (created_at >= TIMESTAMPTZ '1970-01-01 00:00:00+00')
);
-- 来源优先查找，内容始终兜底：两种唯一性同时保护并发写入。
CREATE UNIQUE INDEX flashcards_content_unique ON public.flashcards (user_id, content_hash);
CREATE UNIQUE INDEX flashcards_origin_unique ON public.flashcards (user_id, origin_type, origin_hash)
    WHERE origin_type IS NOT NULL AND origin_id IS NOT NULL;
CREATE INDEX flashcards_due_idx ON public.flashcards (user_id, next_review_at, id);
CREATE INDEX flashcards_list_idx ON public.flashcards (user_id, created_at DESC, id DESC);
CREATE INDEX flashcards_folder_idx ON public.flashcards (user_id, source_folder_id, created_at DESC, id DESC);
CREATE INDEX flashcards_origin_list_idx ON public.flashcards (user_id, origin_type, created_at DESC, id DESC);
CREATE INDEX flashcards_weak_idx ON public.flashcards (user_id, next_review_at, id)
    WHERE ease_factor < 2.5 OR (interval_days <= 3 AND repetitions > 0);
CREATE INDEX flashcards_doc_ref_idx ON public.flashcards (user_id, source_doc_ref_id) WHERE source_doc_ref_id IS NOT NULL;
CREATE INDEX flashcards_folder_ref_idx ON public.flashcards (user_id, source_folder_ref_id) WHERE source_folder_ref_id IS NOT NULL;
COMMENT ON TABLE public.flashcards IS '智能卡片；来源唯一 + 规范内容唯一；SRS 更新和复习记录写入由 service 在同一事务完成';
COMMENT ON COLUMN public.flashcards.content_hash IS '数据库生成 SHA-256 去重键；服务遇到冲突后应比对规范原文，不能把理论上的哈希碰撞当重复';
COMMENT ON COLUMN public.flashcards.source_folder_id IS '历史来源 ID 快照，来源删除后保留；不是存活外键';
COMMENT ON COLUMN public.flashcards.source_folder_ref_id IS '同用户存活资料夹外键，来源删除后置空，历史 ID/名称不变';
COMMENT ON COLUMN public.flashcards.source_doc_id IS '历史文档 ID 快照，文档删除后保留';

-- 7. 快刷题。题型自由文本，对齐前端「选择题 / 简答题 / ...」。
CREATE TABLE public.quick_quizzes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    type text NOT NULL,
    question text NOT NULL,
    options jsonb,
    answer text NOT NULL,
    analysis text NOT NULL DEFAULT '',
    reviewed boolean NOT NULL DEFAULT false,
    source_doc_id uuid,
    source_doc_name text,
    source_folder_id uuid,
    source_folder_name text,
    source_doc_ref_id uuid,
    source_folder_ref_id uuid,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT quick_quizzes_user_id_id_unique UNIQUE (user_id, id),
    CONSTRAINT quick_quizzes_type_check CHECK (char_length(public.medstudy_trim_text(type)) > 0),
    CONSTRAINT quick_quizzes_question_check CHECK (char_length(public.medstudy_trim_text(question)) > 0),
    CONSTRAINT quick_quizzes_options_check CHECK (public.medstudy_valid_options(options)),
    CONSTRAINT quick_quizzes_answer_check CHECK (char_length(public.medstudy_trim_text(answer)) > 0),
    CONSTRAINT quick_quizzes_doc_ref_check CHECK (source_doc_ref_id IS NULL OR (source_doc_id IS NOT NULL AND source_doc_ref_id = source_doc_id)),
    CONSTRAINT quick_quizzes_folder_ref_check CHECK (source_folder_ref_id IS NULL OR (source_folder_id IS NOT NULL AND source_folder_ref_id = source_folder_id)),
    CONSTRAINT quick_quizzes_doc_fk FOREIGN KEY (user_id, source_doc_ref_id)
        REFERENCES public.documents(user_id, id) ON DELETE SET NULL (source_doc_ref_id),
    CONSTRAINT quick_quizzes_folder_fk FOREIGN KEY (user_id, source_folder_ref_id)
        REFERENCES public.folders(user_id, id) ON DELETE SET NULL (source_folder_ref_id),
    CONSTRAINT quick_quizzes_created_at_check CHECK (created_at >= TIMESTAMPTZ '1970-01-01 00:00:00+00')
);
CREATE INDEX quick_quizzes_list_idx ON public.quick_quizzes (user_id, created_at DESC, id DESC);
CREATE INDEX quick_quizzes_reviewed_idx ON public.quick_quizzes (user_id, reviewed, created_at DESC, id DESC);
CREATE INDEX quick_quizzes_folder_idx ON public.quick_quizzes (user_id, source_folder_id, created_at DESC, id DESC);
CREATE INDEX quick_quizzes_doc_ref_idx ON public.quick_quizzes (user_id, source_doc_ref_id) WHERE source_doc_ref_id IS NOT NULL;
CREATE INDEX quick_quizzes_folder_ref_idx ON public.quick_quizzes (user_id, source_folder_ref_id) WHERE source_folder_ref_id IS NOT NULL;
COMMENT ON TABLE public.quick_quizzes IS '持久快刷库；AI 临时考试题和强化练习不单独持久化';

-- 8. 错题。去重仅 trim(question)，不擅自增加大小写/NFKC 规则。
CREATE TABLE public.error_questions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    question text NOT NULL,
    question_hash bytea GENERATED ALWAYS AS (public.medstudy_text_hash(public.medstudy_trim_text(question))) STORED,
    options jsonb,
    wrong_answer text NOT NULL,
    correct_answer text,
    analysis text NOT NULL DEFAULT '',
    source_doc_id uuid,
    source_doc_name text,
    source_folder_id uuid,
    source_folder_name text,
    source_doc_ref_id uuid,
    source_folder_ref_id uuid,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT error_questions_user_id_id_unique UNIQUE (user_id, id),
    CONSTRAINT error_questions_question_check CHECK (char_length(public.medstudy_trim_text(question)) > 0),
    CONSTRAINT error_questions_options_check CHECK (public.medstudy_valid_options(options)),
    CONSTRAINT error_questions_wrong_answer_check CHECK (char_length(public.medstudy_trim_text(wrong_answer)) > 0),
    CONSTRAINT error_questions_doc_ref_check CHECK (source_doc_ref_id IS NULL OR (source_doc_id IS NOT NULL AND source_doc_ref_id = source_doc_id)),
    CONSTRAINT error_questions_folder_ref_check CHECK (source_folder_ref_id IS NULL OR (source_folder_id IS NOT NULL AND source_folder_ref_id = source_folder_id)),
    CONSTRAINT error_questions_doc_fk FOREIGN KEY (user_id, source_doc_ref_id)
        REFERENCES public.documents(user_id, id) ON DELETE SET NULL (source_doc_ref_id),
    CONSTRAINT error_questions_folder_fk FOREIGN KEY (user_id, source_folder_ref_id)
        REFERENCES public.folders(user_id, id) ON DELETE SET NULL (source_folder_ref_id),
    CONSTRAINT error_questions_created_at_check CHECK (created_at >= TIMESTAMPTZ '1970-01-01 00:00:00+00')
);
-- NULL 资料夹也参与去重；历史来源 ID 保留，删除资料夹不会让不同来源的键合并。
CREATE UNIQUE INDEX error_questions_dedup_unique ON public.error_questions
    (user_id, question_hash, source_folder_id) NULLS NOT DISTINCT;
CREATE INDEX error_questions_list_idx ON public.error_questions (user_id, created_at DESC, id DESC);
CREATE INDEX error_questions_folder_idx ON public.error_questions (user_id, source_folder_id, created_at DESC, id DESC);
CREATE INDEX error_questions_doc_ref_idx ON public.error_questions (user_id, source_doc_ref_id) WHERE source_doc_ref_id IS NOT NULL;
CREATE INDEX error_questions_folder_ref_idx ON public.error_questions (user_id, source_folder_ref_id) WHERE source_folder_ref_id IS NOT NULL;
COMMENT ON TABLE public.error_questions IS '错题；关联卡片 ID 由 error_flashcards 查询得到，不存悬空 UUID 数组';

-- 来源 ID/名称是历史快照；ref_id 才是存活外键。
-- 绑定存在的同用户资源，拒绝指向其他用户当前资源；已删除的历史 ID 允许从备份恢复。
-- 新建 API 仍必须由 service 验证来源当前存在，不能把历史恢复规则当作绕过校验的方式。
CREATE FUNCTION public.medstudy_bind_source_refs()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = pg_catalog, public
AS $function$
BEGIN
    IF TG_OP = 'UPDATE' AND NEW.user_id IS DISTINCT FROM OLD.user_id THEN
        RAISE EXCEPTION '资源不能转移到其他用户' USING ERRCODE = '23514';
    END IF;
    IF TG_OP = 'INSERT' OR NEW.source_doc_id IS DISTINCT FROM OLD.source_doc_id THEN
        NEW.source_doc_ref_id := NULL;
        IF NEW.source_doc_id IS NOT NULL THEN
            SELECT id INTO NEW.source_doc_ref_id FROM public.documents
                WHERE user_id = NEW.user_id AND id = NEW.source_doc_id;
            IF NEW.source_doc_ref_id IS NULL AND EXISTS (SELECT 1 FROM public.documents WHERE id = NEW.source_doc_id) THEN
                RAISE EXCEPTION '文档来源不属于当前用户' USING ERRCODE = '23503';
            END IF;
        END IF;
    END IF;
    IF TG_OP = 'INSERT' OR NEW.source_folder_id IS DISTINCT FROM OLD.source_folder_id THEN
        NEW.source_folder_ref_id := NULL;
        IF NEW.source_folder_id IS NOT NULL THEN
            SELECT id INTO NEW.source_folder_ref_id FROM public.folders
                WHERE user_id = NEW.user_id AND id = NEW.source_folder_id;
            IF NEW.source_folder_ref_id IS NULL AND EXISTS (SELECT 1 FROM public.folders WHERE id = NEW.source_folder_id) THEN
                RAISE EXCEPTION '资料夹来源不属于当前用户' USING ERRCODE = '23503';
            END IF;
        END IF;
    END IF;
    RETURN NEW;
END;
$function$;
CREATE TRIGGER flashcards_bind_sources BEFORE INSERT OR UPDATE ON public.flashcards
    FOR EACH ROW EXECUTE FUNCTION public.medstudy_bind_source_refs();
CREATE TRIGGER quick_quizzes_bind_sources BEFORE INSERT OR UPDATE ON public.quick_quizzes
    FOR EACH ROW EXECUTE FUNCTION public.medstudy_bind_source_refs();
CREATE TRIGGER error_questions_bind_sources BEFORE INSERT OR UPDATE ON public.error_questions
    FOR EACH ROW EXECUTE FUNCTION public.medstudy_bind_source_refs();

-- 9. 错题与卡片的多对多关系。两端都必须属于同一 user_id。
CREATE TABLE public.error_flashcards (
    user_id uuid NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    error_id uuid NOT NULL,
    flashcard_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, error_id, flashcard_id),
    CONSTRAINT error_flashcards_error_fk FOREIGN KEY (user_id, error_id)
        REFERENCES public.error_questions(user_id, id) ON DELETE CASCADE,
    CONSTRAINT error_flashcards_card_fk FOREIGN KEY (user_id, flashcard_id)
        REFERENCES public.flashcards(user_id, id) ON DELETE CASCADE
);
CREATE INDEX error_flashcards_card_idx ON public.error_flashcards (user_id, flashcard_id);
COMMENT ON TABLE public.error_flashcards IS '删除错题或卡片只级联删除关联，删除错题不删除卡片';

-- 10. 复习事件。持久卡片有真正外键，临时题 UUID 是事件主体快照。
CREATE TABLE public.review_history (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    subject_id uuid NOT NULL,
    subject_type text NOT NULL,
    flashcard_id uuid GENERATED ALWAYS AS (CASE WHEN subject_type = 'flashcard' THEN subject_id ELSE NULL END) STORED,
    rating text NOT NULL,
    reviewed_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT review_history_subject_type_check CHECK (subject_type IN ('flashcard', 'quickQuiz', 'examQuestion', 'reinforcement')),
    CONSTRAINT review_history_rating_check CHECK (rating IN ('forgot', 'hard', 'good')),
    CONSTRAINT review_history_time_check CHECK (reviewed_at >= TIMESTAMPTZ '1970-01-01 00:00:00+00'),
    CONSTRAINT review_history_flashcard_fk FOREIGN KEY (user_id, flashcard_id)
        REFERENCES public.flashcards(user_id, id) ON DELETE CASCADE
);
CREATE INDEX review_history_time_idx ON public.review_history (user_id, reviewed_at DESC, id DESC);
CREATE INDEX review_history_rating_time_idx ON public.review_history (user_id, rating, reviewed_at DESC, id DESC);
CREATE INDEX review_history_subject_idx ON public.review_history (user_id, subject_type, subject_id, reviewed_at DESC, id DESC);
CREATE INDEX review_history_flashcard_idx ON public.review_history (user_id, flashcard_id) WHERE flashcard_id IS NOT NULL;
COMMENT ON TABLE public.review_history IS 'API cardId 映射 subject_id，timestamp 映射 reviewed_at；持久卡片删除后其复习记录级联删除';
COMMENT ON COLUMN public.review_history.subject_id IS '临时题、已删除快刷题的历史 UUID 允许保留；quickQuiz 写入前由 service 校验归属';

-- 11. 恢复前安全备份。业务安全备份与 PostgreSQL 全库灾备不同。
CREATE TABLE public.safety_backups (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    reason text NOT NULL,
    schema_version integer NOT NULL DEFAULT 1,
    revision bigint NOT NULL,
    snapshot jsonb,
    object_key text,
    summary jsonb NOT NULL,
    size_bytes bigint NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT safety_backups_reason_check CHECK (reason IN ('conflict', 'before-restore')),
    CONSTRAINT safety_backups_schema_check CHECK (schema_version > 0),
    CONSTRAINT safety_backups_revision_check CHECK (revision >= 0),
    CONSTRAINT safety_backups_storage_check CHECK ((snapshot IS NULL) <> (object_key IS NULL)),
    CONSTRAINT safety_backups_snapshot_check CHECK (snapshot IS NULL OR jsonb_typeof(snapshot) = 'object'),
    CONSTRAINT safety_backups_object_key_check CHECK (object_key IS NULL OR char_length(public.medstudy_trim_text(object_key)) > 0),
    CONSTRAINT safety_backups_summary_check CHECK (jsonb_typeof(summary) = 'object'),
    CONSTRAINT safety_backups_size_check CHECK (size_bytes >= 0)
);
CREATE INDEX safety_backups_list_idx ON public.safety_backups (user_id, created_at DESC, id DESC);
COMMENT ON TABLE public.safety_backups IS '完整快照或受控对象存储 key 二选一；API source 返回 server；service 保留至少最近 3 份恢复前备份';

-- 12. 幂等请求。相同用户 + 路由 + key 唯一；过期行由任务清理/受锁替换。
CREATE TABLE public.idempotency_keys (
    user_id uuid NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    route text COLLATE "C" NOT NULL,
    key text COLLATE "C" NOT NULL,
    request_hash bytea NOT NULL,
    status text NOT NULL DEFAULT 'processing',
    response_status integer,
    response_headers jsonb,
    response jsonb,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at timestamptz NOT NULL DEFAULT (CURRENT_TIMESTAMP + INTERVAL '24 hours'),
    PRIMARY KEY (user_id, route, key),
    CONSTRAINT idempotency_keys_route_check CHECK (char_length(route) BETWEEN 1 AND 255),
    CONSTRAINT idempotency_keys_key_check CHECK (char_length(key) BETWEEN 8 AND 128),
    CONSTRAINT idempotency_keys_hash_check CHECK (octet_length(request_hash) = 32),
    CONSTRAINT idempotency_keys_status_check CHECK (status IN ('processing', 'completed')),
    CONSTRAINT idempotency_keys_response_status_check CHECK (response_status BETWEEN 100 AND 599),
    CONSTRAINT idempotency_keys_response_headers_check CHECK (response_headers IS NULL OR jsonb_typeof(response_headers) = 'object'),
    CONSTRAINT idempotency_keys_completion_check CHECK (
        (status = 'processing' AND response_status IS NULL AND response_headers IS NULL AND response IS NULL)
        OR (status = 'completed' AND response_status IS NOT NULL AND response_headers IS NOT NULL AND response IS NOT NULL)),
    CONSTRAINT idempotency_keys_expiry_check CHECK (expires_at > created_at)
);
CREATE INDEX idempotency_keys_expiry_idx ON public.idempotency_keys (expires_at);
COMMENT ON TABLE public.idempotency_keys IS '业务写入与完成响应须在同一事务提交；不可保存 token、Set-Cookie 或 AI Key 到响应缓存';
COMMENT ON COLUMN public.idempotency_keys.route IS '含 HTTP 方法及规范资源路径，例如 POST /api/v1/flashcards/{实际UUID}/reviews，不能跨资源复用结果';
COMMENT ON COLUMN public.idempotency_keys.request_hash IS '规范请求体的 SHA-256；同 key 不同请求体由 service 返回 409';

-- 13. 备份/旧快照 preview 的短期一次性 token 与加密暂存。
-- 补充契约要求的持久状态，不改变 HTTP 字段；定期清理过期/已使用记录。
CREATE TABLE public.import_previews (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    purpose text NOT NULL,
    token_hash bytea NOT NULL,
    payload_hash bytea NOT NULL,
    ciphertext bytea NOT NULL,
    nonce bytea NOT NULL,
    key_version integer NOT NULL,
    summary jsonb NOT NULL,
    warnings jsonb NOT NULL DEFAULT '[]'::jsonb,
    expires_at timestamptz NOT NULL,
    used_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT import_previews_token_hash_unique UNIQUE (token_hash),
    CONSTRAINT import_previews_purpose_check CHECK (purpose IN ('backup_import', 'legacy_import')),
    CONSTRAINT import_previews_token_hash_check CHECK (octet_length(token_hash) = 32),
    CONSTRAINT import_previews_payload_hash_check CHECK (octet_length(payload_hash) = 32),
    CONSTRAINT import_previews_ciphertext_check CHECK (octet_length(ciphertext) >= 16),
    CONSTRAINT import_previews_nonce_check CHECK (octet_length(nonce) = 12),
    CONSTRAINT import_previews_key_version_check CHECK (key_version > 0),
    CONSTRAINT import_previews_summary_check CHECK (jsonb_typeof(summary) = 'object'),
    CONSTRAINT import_previews_warnings_check CHECK (jsonb_typeof(warnings) = 'array'),
    CONSTRAINT import_previews_expiry_check CHECK (expires_at > created_at)
);
CREATE INDEX import_previews_expiry_idx ON public.import_previews (expires_at);
CREATE INDEX import_previews_user_idx ON public.import_previews (user_id, created_at DESC, id DESC);
COMMENT ON TABLE public.import_previews IS '只存 token 哈希和 AES-GCM 密文；账号、purpose、payload_hash、有效期及一次性消费由 backup service 联合校验';

-- 自动更新时间不替代业务事务，也不自动递增用户 revision。
DO $triggers$
DECLARE
    target_table text;
BEGIN
    FOREACH target_table IN ARRAY ARRAY['users', 'ai_credentials', 'folders', 'documents', 'flashcards', 'quick_quizzes', 'error_questions']
    LOOP
        EXECUTE format('CREATE TRIGGER %I BEFORE UPDATE ON public.%I FOR EACH ROW EXECUTE FUNCTION public.medstudy_touch_updated_at()',
            target_table || '_touch_updated_at', target_table);
    END LOOP;
END;
$triggers$;

INSERT INTO public.schema_migrations (version, name) VALUES (1, 'initial_schema');
COMMIT;
