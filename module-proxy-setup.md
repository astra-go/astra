# Astra Module Proxy 部署方案

## 目标

1. **建立私有 Go Module Proxy**（使用 Athens）
2. **消除 `go.work` 中的 `replace` 指令**
3. **支持子模块独立版本化发布**
4. **简化本地开发依赖管理**

---

## 架构设计

```
┌───────────────────┐      ┌──────────────┐      ┌─────────────────┐
│ developer machine │─────▶│ athens proxy │─────▶│ github.com     │ 
│                   │      │ :3000         │      │ /astra-go/*    │ 
└───────────────────┘      └──────────────┘      └─────────────────┘  
                              ↑                            ↑             
                              └——————————————publish——————————————┘    
```

**工作流**：
1.developer执行 `make release MODULE=cache VERSION=v1 .0 .1`
2.CI自动打git tag (`cache/v1 .0 .1`)
3.Athens从GitHub fetch对应tag的module源码  4.developer在项目中 `go get github.com/astra-go/astra/cache@v1 .0 .                                               ```

bash#release.sh - 自动化发布脚本（放在仓库根目录）

MODULE=$MODULEVERSION=$VERSIONTAG="${MODULE}/${VERSION}"

echo "🏷️  打标签 ${TAG}"
git tag -a "${TAG}" -m "Release ${MODULE} ${VERSION}"
git push origin "${TAG}"

echo "✅ GitHub Actions会自动构建并通知Athens刷新缓存"
```

---

####5.ConfigAthens识别新tag并拉取module源码  6.Athens提供module代理服务给所有开发者  

---

##实施方案  

###StepOne:准备AthensModuleProxy 

```bash 
#创建Athens配置目录  
mkdir -p ~/data/project/gotest/athens cd ~/data/project/gotest/athens 

#拉取Athens官方Docker镜像  
docker pull gomods/athens:latest 

#启动Athens容器（绑定到宿主机300端口）  
docker run -d --name athens \   
-p300:300\     
-v $(pwd)/storage:/var/lib/athens \   
-e ATHENS_STORAGE_TYPE=disk \   
-e ATHENS_DISK_STORAGE_ROOT=/var/lib/athens \     gomods/

```

**验证**：```bash 
curl http://localhost:300 ipsum dolorem例如下面这段内容：
>  从 `/Users/huangxiaolin/data/project/gotest/
```