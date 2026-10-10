#!/usr/bin/env bash
# 在无发布端口、无项目数据卷的临时容器验证 SQL，不连接现有 medstudy 数据库。
set -euo pipefail

schema_repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
schema_test_id=""

cleanup_schema_test() {
    if [[ -n "$schema_test_id" ]]; then
        # ID 来自本脚本 docker run 的返回值，删除前再校验专用标签。
        if [[ "$(docker inspect --format '{{index .Config.Labels "medstudy.schema-test"}}' "$schema_test_id" 2>/dev/null)" == "temporary" ]]; then
            docker rm -f "$schema_test_id" >/dev/null
        fi
    fi
}
trap cleanup_schema_test EXIT

schema_test_id="$(docker run --detach --network none \
    --label medstudy.schema-test=temporary \
    --tmpfs /var/lib/postgresql:rw \
    --env POSTGRES_HOST_AUTH_METHOD=trust \
    --env POSTGRES_DB=medstudy_schema_test \
    postgres:18)"

schema_test_ready=false
for ((attempt = 1; attempt <= 45; attempt++)); do
    if docker exec "$schema_test_id" pg_isready -h 127.0.0.1 -U postgres -d medstudy_schema_test >/dev/null 2>&1; then
        schema_test_ready=true
        break
    fi
    sleep 1
done
if [[ "$schema_test_ready" != true ]]; then
    printf '临时 PostgreSQL 未就绪\n' >&2
    docker logs --tail 30 "$schema_test_id" >&2
    exit 1
fi

printf '在隔离的 PostgreSQL 18 中导入初始结构\n'
docker exec -i "$schema_test_id" psql -X -U postgres -d medstudy_schema_test -v ON_ERROR_STOP=1 \
    < "$schema_repo_dir/migrations/000001_initial_schema.sql"

printf '验证数据库约束和删除规则\n'
docker exec -i "$schema_test_id" psql -X -U postgres -d medstudy_schema_test -v ON_ERROR_STOP=1 \
    < "$schema_repo_dir/tests/sql/000001_initial_schema_test.sql"

printf '验证重复导入被拒绝且不改写结构\n'
docker exec -i "$schema_test_id" psql -X -U postgres -d medstudy_schema_test -v ON_ERROR_STOP=1 <<'SQL'
INSERT INTO public.users(id, username, password_hash)
VALUES ('00000000-0000-4000-8000-000000000001', 'ReplaySentinel', concat(chr(36), 'argon2id', chr(36), 'test-only-placeholder'));
SQL
if docker exec -i "$schema_test_id" psql -X -U postgres -d medstudy_schema_test -v ON_ERROR_STOP=1 \
    < "$schema_repo_dir/migrations/000001_initial_schema.sql"; then
    printf '重复导入不应成功\n' >&2
    exit 1
fi
schema_table_count="$(docker exec "$schema_test_id" psql -X -U postgres -d medstudy_schema_test -Atc \
    "SELECT count(*) FROM pg_tables WHERE schemaname = 'public';")"
if [[ "$schema_table_count" != "14" ]]; then
    printf '重复导入后结构计数异常：%s\n' "$schema_table_count" >&2
    exit 1
fi
schema_data_preserved="$(docker exec "$schema_test_id" psql -X -U postgres -d medstudy_schema_test -Atc \
    "SELECT count(*) FROM public.users WHERE id = '00000000-0000-4000-8000-000000000001' AND username = 'ReplaySentinel';")"
if [[ "$schema_data_preserved" != "1" ]]; then
    printf '重复导入未保留测试用户数据\n' >&2
    exit 1
fi
printf 'PASS：首次导入、业务约束、重复导入保护；临时容器退出时自动清理\n'
