# 自有版本云更新（Docker Compose）

更新源是 `zz456456/sub2`，镜像是 `ghcr.io/zz456456/sub2`。后台只检查这个仓库的正式 Release；原项目的 Release 不再参与版本检查，旧的上游更新缓存也不会沿用。

## 发布你的版本

代码提交到 `main` 后，为准备发布的提交创建一个递增的版本标签，例如当前 `0.2.19` 的下一个版本可以使用 `v0.2.20`：

```bash
git tag -a v0.2.20 -m "发布自定义功能"
git push https://github.com/zz456456/sub2.git main refs/tags/v0.2.20
```

仓库的 **Actions → Release** 会构建前端和程序，发布 GitHub Release 以及 Docker 镜像。默认包含 Linux amd64 / arm64 镜像。无需配置 Docker Hub 账号；发布 GHCR 和 Release 使用工作流自带的 `GITHUB_TOKEN`。

也可在 Actions 中手动运行 Release，`tag` 填已存在的版本标签。只需要 Linux amd64 时勾选 `simple_release`。勾选 `dry_run` 可以验证构建但不发布，此时 `tag` 可填 `main`。

正式版本会更新 `latest`。包含 `-` 的预发布版本（例如 `v0.2.20-rc.1`）只发布对应版本标签，不覆盖正式版本的 `latest`。

首次发布后，在 GitHub 用户页的 **Packages → sub2 → Package settings** 检查镜像可见性。需要匿名拉取时将包设置为 Public；公开代码仓库不会自动使新建的 GHCR 包公开。保留私有镜像时，先在部署服务器用具有 `read:packages` 权限的账号执行 `docker login ghcr.io`。

## 让现有部署使用你的镜像

在服务器的部署目录中，将 Compose 的应用服务镜像改为：

```yaml
services:
  sub2api:
    image: ${SUB2API_IMAGE:-ghcr.io/zz456456/sub2:latest}
```

仓库中的三个生产 Compose 文件已使用这个默认值。`.env` 中可配置：

```dotenv
SUB2API_IMAGE=ghcr.io/zz456456/sub2:latest
```

第一次启用本功能时也需要执行下面的更新命令，以安装包含自有更新源的新镜像。

## 更新已部署的服务

确认 Actions 的 Release 已成功，并在服务器部署目录执行：

```bash
docker compose pull sub2api
docker compose up -d --no-deps sub2api
```

如部署时使用了其他 Compose 文件，两条命令都要指定同一个文件，例如：

```bash
docker compose -f docker-compose.local.yml pull sub2api
docker compose -f docker-compose.local.yml up -d --no-deps sub2api
```

Docker 重建应用容器并复用现有数据卷。后台左上角的版本按钮会检查你发布的新版本，显示更新提示、发布页面和可复制的更新命令。检查失败或仓库尚无正式 Release 时会显示原因。

云更新采用“你发布版本，后台提示，服务器执行更新”的方式。网页不会操作 Docker 守护进程；更新和回退的二进制替换 API 会明确要求使用 Docker Compose。

GitHub API 限流时可在部署的 `.env` 中设置 `UPDATE_GITHUB_TOKEN`，然后重建应用容器。该变量只用于版本检查，与 `docker login` 的镜像拉取凭据分开配置。

## 固定版本与回退

在 `.env` 中把镜像改为已发布的版本，例如 `SUB2API_IMAGE=ghcr.io/zz456456/sub2:0.2.20`，然后执行相同的 `pull` 和 `up` 命令。固定标签后，后续发布不会改变此部署使用的版本；恢复跟随正式版本时改回 `latest`。
