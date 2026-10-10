-- 仅在 scripts/verify_schema.sh 创建的隔离测试容器中执行。
-- 测试数据在事务末尾回滚，不要在 Navicat 中导入此测试文件。
BEGIN;

-- 失败场景助手：执行语句，检查它确实失败且错误代码符合预期；意外成功也算失败。
-- 23505 表示唯一约束冲突，23503 表示外键约束冲突，23514 表示检查约束不满足。
CREATE FUNCTION pg_temp.expect_sqlstate(statement text, expected_state text)
RETURNS void LANGUAGE plpgsql AS $function$
DECLARE
    actual_state text;
BEGIN
    BEGIN
        EXECUTE statement;
    EXCEPTION WHEN OTHERS THEN
        GET STACKED DIAGNOSTICS actual_state = RETURNED_SQLSTATE;
        IF actual_state <> expected_state THEN
            RAISE EXCEPTION '期望 SQLSTATE %，实际 %，语句 %', expected_state, actual_state, statement;
        END IF;
        RETURN;
    END;
    RAISE EXCEPTION '期望 SQLSTATE %，语句却执行成功：%', expected_state, statement;
END;
$function$;

DO $tests$
DECLARE
    owner_id uuid;
    other_id uuid;
    folder_id uuid;
    second_folder_id uuid;
    foreign_folder_id uuid;
    document_id uuid;
    foreign_document_id uuid;
    card_id uuid;
    other_card_id uuid;
    error_id uuid;
    quiz_id uuid;
    session_id uuid;
    foreign_session_id uuid;
    count_value integer;
BEGIN
    -- 结构验收：应有 14 张表，并记录初始迁移版本 1。
    SELECT count(*) INTO count_value FROM pg_tables WHERE schemaname = 'public';
    IF count_value <> 14 THEN RAISE EXCEPTION '期望 14 张表，实际 %', count_value; END IF;
    IF NOT EXISTS (SELECT 1 FROM public.schema_migrations WHERE version = 1 AND name = 'initial_schema') THEN
        RAISE EXCEPTION '缺少迁移版本记录';
    END IF;
    -- 文本规则：全角英文、大小写和不同空白应统一；不同 front/back 组合不能因拼接产生同一哈希。
    IF public.medstudy_normalize_text(U&'\3000\FF25\FF23\FF27\0009  检查\000A') <> 'ecg 检查' THEN
        RAISE EXCEPTION 'NFKC/大小写/Unicode 空白规范化错误';
    END IF;
    IF public.medstudy_flashcard_content_hash('ab', 'c') = public.medstudy_flashcard_content_hash('a', 'bc') THEN
        RAISE EXCEPTION 'front/back 哈希拼接有歧义';
    END IF;

    -- 用户规则：大小写/全角变体不能重复注册；明文密码与过短用户名应被拒绝。
    -- 这些是测试占位哈希，不是实际可登录账号；全部会回滚。
    INSERT INTO public.users(username, password_hash) VALUES ('TestUser', '$argon2id$test-only-placeholder') RETURNING id INTO owner_id;
    INSERT INTO public.users(username, password_hash) VALUES ('OtherUser', '$argon2id$test-only-placeholder') RETURNING id INTO other_id;
    PERFORM pg_temp.expect_sqlstate('INSERT INTO public.users(username, password_hash) VALUES (''testuser'', ''$argon2id$test'')', '23505');
    PERFORM pg_temp.expect_sqlstate('INSERT INTO public.users(username, password_hash) VALUES (U&''\FF34\FF25\FF33\FF34\FF35\FF33\FF25\FF32'', ''$argon2id$test'')', '23505');
    PERFORM pg_temp.expect_sqlstate('INSERT INTO public.users(username, password_hash) VALUES (''abc'', ''plaintext'')', '23514');
    PERFORM pg_temp.expect_sqlstate('INSERT INTO public.users(username, password_hash) VALUES (''ab'', ''$argon2id$test'')', '23514');

    -- 资料夹规则：允许两种类型及 100 个汉字；拒绝 101 字、未知类型、纯空白及未去除的前置空白。
    INSERT INTO public.folders(user_id, name, type) VALUES (owner_id, '生理学', 'material') RETURNING id INTO folder_id;
    INSERT INTO public.folders(user_id, name, type) VALUES (owner_id, '生理学', 'exam') RETURNING id INTO second_folder_id;
    INSERT INTO public.folders(user_id, name, type) VALUES (other_id, '其他用户资料', 'material') RETURNING id INTO foreign_folder_id;
    INSERT INTO public.folders(user_id, name, type) VALUES (owner_id, repeat('医', 100), 'material');
    PERFORM pg_temp.expect_sqlstate(format('INSERT INTO public.folders(user_id,name,type) VALUES (%L,%L,''material'')', owner_id, repeat('医', 101)), '23514');
    PERFORM pg_temp.expect_sqlstate(format('INSERT INTO public.folders(user_id,name,type) VALUES (%L,''资料'',''other'')', owner_id), '23514');
    PERFORM pg_temp.expect_sqlstate(format('INSERT INTO public.folders(user_id,name,type) VALUES (%L,E'' \t\n '',''material'')', owner_id), '23514');
    PERFORM pg_temp.expect_sqlstate(format('INSERT INTO public.folders(user_id,name,type) VALUES (%L,E''\t资料'',''material'')', owner_id), '23514');

    -- 文档规则：字符数按汉字数量计算；允许无资料夹的历史文档，拒绝跨用户资料夹和不足十字的正文。
    INSERT INTO public.documents(user_id, folder_id, name, content)
        VALUES (owner_id, folder_id, '循环系统.txt', repeat('医', 10)) RETURNING id INTO document_id;
    INSERT INTO public.documents(user_id, folder_id, name, content)
        VALUES (other_id, foreign_folder_id, '外部资料.txt', repeat('医', 10)) RETURNING id INTO foreign_document_id;
    INSERT INTO public.documents(user_id, folder_id, name, content) VALUES (owner_id, NULL, '历史孤立资料.txt', repeat('医', 10));
    IF (SELECT character_count FROM public.documents WHERE id = document_id) <> 10 THEN
        RAISE EXCEPTION '字符数错误，不应按 UTF8 字节计算';
    END IF;
    PERFORM pg_temp.expect_sqlstate(format('INSERT INTO public.documents(user_id,folder_id,name,content) VALUES (%L,%L,''x.txt'',repeat(''医'',10))', owner_id, foreign_folder_id), '23503');
    PERFORM pg_temp.expect_sqlstate(format('INSERT INTO public.documents(user_id,name,content) VALUES (%L,''x.txt'',''不足十字'')', owner_id), '23514');

    -- 卡片规则：合法来源应绑定存活外键；跨用户来源、过小或 NaN 的难度系数应被拒绝。
    -- 相同内容允许属于不同用户；同一用户的来源和内容去重分别受到保护。
    INSERT INTO public.flashcards(user_id, front, back, origin_type, origin_id, source_doc_id, source_doc_name, source_folder_id, source_folder_name)
        VALUES (owner_id, ' ECG 检查 ', '心电图', 'exam', 'origin-one', document_id, '循环系统.txt', folder_id, '生理学') RETURNING id INTO card_id;
    IF NOT EXISTS (SELECT 1 FROM public.flashcards WHERE id = card_id AND source_doc_ref_id = document_id AND source_folder_ref_id = folder_id) THEN
        RAISE EXCEPTION '来源存活外键未自动绑定';
    END IF;
    -- 同来源不同内容、不同来源同内容、无来源同内容都不允许重复。
    PERFORM pg_temp.expect_sqlstate(format('INSERT INTO public.flashcards(user_id,front,back,origin_type,origin_id) VALUES (%L,''其他题'',''其他答案'',''exam'',''origin-one'')', owner_id), '23505');
    PERFORM pg_temp.expect_sqlstate(format('INSERT INTO public.flashcards(user_id,front,back,origin_type,origin_id) VALUES (%L,E''ecg\t检查'',''心电图'',''exam'',''origin-two'')', owner_id), '23505');
    PERFORM pg_temp.expect_sqlstate(format('INSERT INTO public.flashcards(user_id,front,back) VALUES (%L,''ecg 检查'',''心电图'')', owner_id), '23505');
    PERFORM pg_temp.expect_sqlstate(format('INSERT INTO public.flashcards(user_id,front,back,source_folder_id) VALUES (%L,''跨用户来源题'',''答案'',%L)', owner_id, foreign_folder_id), '23503');
    PERFORM pg_temp.expect_sqlstate(format('INSERT INTO public.flashcards(user_id,front,back,source_doc_id) VALUES (%L,''跨用户文档题'',''答案'',%L)', owner_id, foreign_document_id), '23503');
    PERFORM pg_temp.expect_sqlstate(format('INSERT INTO public.flashcards(user_id,front,back,ease_factor) VALUES (%L,''题目'',''答案'',1.2)', owner_id), '23514');
    PERFORM pg_temp.expect_sqlstate(format('INSERT INTO public.flashcards(user_id,front,back,ease_factor) VALUES (%L,''题目'',''答案'',''NaN'')', owner_id), '23514');
    INSERT INTO public.flashcards(user_id, front, back) VALUES (other_id, 'ecg 检查', '心电图') RETURNING id INTO other_card_id;
    -- 长正文进入哈希索引，不能遇到 B-tree 单条索引键大小限制。
    INSERT INTO public.flashcards(user_id, front, back) VALUES (owner_id, repeat('长题', 10000), repeat('长答', 10000));

    -- 快刷题规则：选项应为字符串值组成的 JSON 对象；数组、数字选项值和跨用户来源应被拒绝。
    INSERT INTO public.quick_quizzes(user_id, type, question, options, answer, source_folder_id, source_folder_name)
        VALUES (owner_id, '选择题', '测试题干', '{"A":"选项一","B":"选项二"}', 'A', folder_id, '生理学') RETURNING id INTO quiz_id;
    PERFORM pg_temp.expect_sqlstate(format('INSERT INTO public.quick_quizzes(user_id,type,question,options,answer) VALUES (%L,''选择题'',''题干'',''[]'',''A'')', owner_id), '23514');
    PERFORM pg_temp.expect_sqlstate(format('INSERT INTO public.quick_quizzes(user_id,type,question,options,answer) VALUES (%L,''选择题'',''题干'',''{"A":123}'',''A'')', owner_id), '23514');
    PERFORM pg_temp.expect_sqlstate(format('INSERT INTO public.quick_quizzes(user_id,type,question,answer,source_doc_id) VALUES (%L,''简答题'',''题干'',''答案'',%L)', owner_id, foreign_document_id), '23503');

    -- 错题规则：同用户、同来源、相同规范化题干不能重复，无来源时也要去重；不同来源允许并存。
    -- 错题来源和错题-卡片关联都不能跨用户。
    INSERT INTO public.error_questions(user_id, question, wrong_answer, source_folder_id, source_folder_name)
        VALUES (owner_id, '错题题干', 'B', folder_id, '生理学') RETURNING id INTO error_id;
    INSERT INTO public.error_questions(user_id, question, wrong_answer, source_folder_id) VALUES (owner_id, '错题题干', 'B', second_folder_id);
    INSERT INTO public.error_questions(user_id, question, wrong_answer) VALUES (owner_id, '无来源错题', 'B');
    PERFORM pg_temp.expect_sqlstate(format('INSERT INTO public.error_questions(user_id,question,wrong_answer) VALUES (%L,'' 无来源错题 '',''B'')', owner_id), '23505');
    PERFORM pg_temp.expect_sqlstate(format('INSERT INTO public.error_questions(user_id,question,wrong_answer,source_folder_id) VALUES (%L,'' 错题题干 '',''B'',%L)', owner_id, folder_id), '23505');
    PERFORM pg_temp.expect_sqlstate(format('INSERT INTO public.error_questions(user_id,question,wrong_answer,source_folder_id) VALUES (%L,''跨账号错题'',''B'',%L)', owner_id, foreign_folder_id), '23503');
    INSERT INTO public.error_flashcards(user_id, error_id, flashcard_id) VALUES (owner_id, error_id, card_id);
    PERFORM pg_temp.expect_sqlstate(format('INSERT INTO public.error_flashcards(user_id,error_id,flashcard_id) VALUES (%L,%L,%L)', owner_id, error_id, other_card_id), '23503');

    -- 复习记录：持久卡片须属于当前用户，临时考试题允许独立 ID；非法评分应被拒绝。
    INSERT INTO public.review_history(user_id, subject_id, subject_type, rating) VALUES (owner_id, card_id, 'flashcard', 'good');
    INSERT INTO public.review_history(user_id, subject_id, subject_type, rating) VALUES (owner_id, gen_random_uuid(), 'examQuestion', 'hard');
    PERFORM pg_temp.expect_sqlstate(format('INSERT INTO public.review_history(user_id,subject_id,subject_type,rating) VALUES (%L,%L,''flashcard'',''good'')', owner_id, other_card_id), '23503');
    PERFORM pg_temp.expect_sqlstate(format('INSERT INTO public.review_history(user_id,subject_id,subject_type,rating) VALUES (%L,%L,''flashcard'',''bad'')', owner_id, card_id), '23514');

    -- 会话规则：token 哈希不能重复，旧会话不能指向其他用户的新会话。
    INSERT INTO public.sessions(user_id, token_hash, expires_at) VALUES (owner_id, sha256('token-one'::bytea), CURRENT_TIMESTAMP + INTERVAL '1 day') RETURNING id INTO session_id;
    INSERT INTO public.sessions(user_id, token_hash, expires_at) VALUES (other_id, sha256('token-two'::bytea), CURRENT_TIMESTAMP + INTERVAL '1 day') RETURNING id INTO foreign_session_id;
    PERFORM pg_temp.expect_sqlstate(format('INSERT INTO public.sessions(user_id,token_hash,expires_at) VALUES (%L,sha256(''token-one''::bytea),CURRENT_TIMESTAMP+INTERVAL ''1 day'')', owner_id), '23505');
    PERFORM pg_temp.expect_sqlstate(format('UPDATE public.sessions SET consumed_at=CURRENT_TIMESTAMP,replaced_by_id=%L WHERE id=%L', foreign_session_id, session_id), '23503');

    -- 幂等记录：同用户同路由的 key 不能重复；标记完成时必须同时保存响应信息。
    INSERT INTO public.idempotency_keys(user_id, route, key, request_hash)
        VALUES (owner_id, 'POST /api/v1/flashcards/bulk', 'test-key-123', sha256('body'::bytea));
    PERFORM pg_temp.expect_sqlstate(format('INSERT INTO public.idempotency_keys(user_id,route,key,request_hash) VALUES (%L,''POST /api/v1/flashcards/bulk'',''test-key-123'',sha256(''body''::bytea))', owner_id), '23505');
    PERFORM pg_temp.expect_sqlstate(format('UPDATE public.idempotency_keys SET status=''completed'' WHERE user_id=%L', owner_id), '23514');
    UPDATE public.idempotency_keys SET status = 'completed', response_status = 201, response_headers = '{}', response = '{"data":{}}' WHERE user_id = owner_id;
    -- 安全备份：内联快照与对象存储 key 必须二选一，不能都缺失或同时存在。
    INSERT INTO public.safety_backups(user_id, reason, revision, snapshot, summary, size_bytes)
        VALUES (owner_id, 'before-restore', 0, '{"format":"medstudy-backup"}', '{}', 32);
    PERFORM pg_temp.expect_sqlstate(format('INSERT INTO public.safety_backups(user_id,reason,revision,summary,size_bytes) VALUES (%L,''before-restore'',0,''{}'',0)', owner_id), '23514');
    PERFORM pg_temp.expect_sqlstate(format('INSERT INTO public.safety_backups(user_id,reason,revision,snapshot,object_key,summary,size_bytes) VALUES (%L,''before-restore'',0,''{}'',''key'',''{}'',0)', owner_id), '23514');
    -- 凭据结构：12 字节 nonce 的占位密文可保存，1 字节 nonce 应被拒绝；不测试真实加解密。
    INSERT INTO public.ai_credentials(user_id, ciphertext, nonce, key_version, last4)
        VALUES (owner_id, decode(repeat('00', 32), 'hex'), decode(repeat('00', 12), 'hex'), 1, 'abcd');
    PERFORM pg_temp.expect_sqlstate(format('INSERT INTO public.ai_credentials(user_id,ciphertext,nonce,key_version,last4) VALUES (%L,decode(repeat(''00'',32),''hex''),decode(''00'',''hex''),1,''abcd'')', other_id), '23514');
    -- 导入预览：准备一条合法的临时预览，用于后面的用户删除级联检查。
    INSERT INTO public.import_previews(user_id, purpose, token_hash, payload_hash, ciphertext, nonce, key_version, summary, expires_at)
        VALUES (owner_id, 'backup_import', sha256('preview-token'::bytea), sha256('payload'::bytea), decode(repeat('00',32),'hex'), decode(repeat('00',12),'hex'), 1, '{}', CURRENT_TIMESTAMP + INTERVAL '5 minutes');

    -- 删除资料夹：文档级联删，卡片/错题/快刷仍在，历史来源 ID/名称仍在。
    DELETE FROM public.folders WHERE id = folder_id;
    IF EXISTS (SELECT 1 FROM public.documents WHERE id = document_id) THEN RAISE EXCEPTION '资料夹删除没有级联删除文档'; END IF;
    IF NOT EXISTS (SELECT 1 FROM public.flashcards WHERE id = card_id AND source_folder_id = folder_id AND source_folder_name = '生理学'
        AND source_doc_id = document_id AND source_doc_ref_id IS NULL AND source_folder_ref_id IS NULL) THEN
        RAISE EXCEPTION '卡片来源快照或存活外键清理错误';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM public.error_questions WHERE id = error_id AND source_folder_id = folder_id AND source_folder_ref_id IS NULL) THEN
        RAISE EXCEPTION '错题被误删或来源快照丢失';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM public.quick_quizzes WHERE id = quiz_id AND source_folder_id = folder_id AND source_folder_ref_id IS NULL) THEN
        RAISE EXCEPTION '快刷题被误删或来源快照丢失';
    END IF;
    -- 对卡片的普通修改不会重新绑定已删除来源。
    UPDATE public.flashcards SET back = '心电活动记录' WHERE id = card_id;
    DELETE FROM public.folders WHERE id = second_folder_id;
    SELECT count(*) INTO count_value FROM public.error_questions WHERE user_id = owner_id AND question = '错题题干';
    IF count_value <> 2 THEN RAISE EXCEPTION '删除资料夹导致不同来源错题错误合并'; END IF;

    -- 删除错题不删除卡片；删除卡片删除卡片事件，临时题事件保留。
    DELETE FROM public.error_questions WHERE id = error_id;
    IF NOT EXISTS (SELECT 1 FROM public.flashcards WHERE id = card_id) THEN RAISE EXCEPTION '删除错题误删卡片'; END IF;
    IF EXISTS (SELECT 1 FROM public.error_flashcards WHERE user_id = owner_id) THEN
        RAISE EXCEPTION '删除错题未清理关联';
    END IF;
    DELETE FROM public.flashcards WHERE id = card_id;
    IF EXISTS (SELECT 1 FROM public.review_history WHERE user_id = owner_id AND subject_type = 'flashcard') THEN
        RAISE EXCEPTION '删除卡片未清理复习记录';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM public.review_history WHERE user_id = owner_id AND subject_type = 'examQuestion') THEN
        RAISE EXCEPTION '临时考试复习记录被误删';
    END IF;

    -- 删除用户：检查下列关联表已清理，同时另一个用户及其卡片仍然存在。
    DELETE FROM public.users WHERE id = owner_id;
    IF EXISTS (SELECT 1 FROM public.sessions WHERE user_id = owner_id)
        OR EXISTS (SELECT 1 FROM public.documents WHERE user_id = owner_id)
        OR EXISTS (SELECT 1 FROM public.flashcards WHERE user_id = owner_id)
        OR EXISTS (SELECT 1 FROM public.quick_quizzes WHERE user_id = owner_id)
        OR EXISTS (SELECT 1 FROM public.error_questions WHERE user_id = owner_id)
        OR EXISTS (SELECT 1 FROM public.review_history WHERE user_id = owner_id)
        OR EXISTS (SELECT 1 FROM public.safety_backups WHERE user_id = owner_id)
        OR EXISTS (SELECT 1 FROM public.ai_credentials WHERE user_id = owner_id)
        OR EXISTS (SELECT 1 FROM public.idempotency_keys WHERE user_id = owner_id)
        OR EXISTS (SELECT 1 FROM public.import_previews WHERE user_id = owner_id) THEN
        RAISE EXCEPTION '删除用户未清理其业务数据';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM public.users WHERE id = other_id)
        OR NOT EXISTS (SELECT 1 FROM public.flashcards WHERE id = other_card_id) THEN
        RAISE EXCEPTION '删除用户影响了其他用户';
    END IF;
    RAISE NOTICE 'PASS：表/版本、Unicode、两种去重、跨用户约束、JSON、凭据结构、幂等、历史来源和级联删除检查';
END;
$tests$;

ROLLBACK;
