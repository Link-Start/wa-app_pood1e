# `/v2/code` `no_routes` 排查记录

日期：2026-06-17

## 结论

当前证据显示：`/v2/code` 的 `no_routes` **不是因为缺少 `/v2/exist` 预热请求导致的**。

在稳定出口代理下，用同一套 `device-ghcr-defaults` 参数底座做对照：

| 对照组 | 样本数 | `sent` | `no_routes` | `blocked` | `/v2/exist` 结果 |
| --- | ---: | ---: | ---: | ---: | --- |
| `code_only` | 12 | 0 | 8 | 4 | 未请求 |
| `exist_then_code` | 12 | 0 | 9 | 3 | 12 个都是 `incorrect` |

`exist_then_code` 没有提升 `sent`，也没有降低 `no_routes`。因此后续排查不应继续把 `/v2/exist` 缺失作为主因。

## 实验边界

- 所有样本只验证注册验证码请求链路，不包含 OTP 提交或注册完成。
- 结果记录只保留状态、原因、聚合计数、号码前缀和脱敏 hash。
- 文档不记录代理账号、代理密码、原始出口 IP、验证码、token、cookie、会话材料、原始 WAMSYS 或完整请求体。
- 临时结果文件位于本地 `.temp/wa-code-param-experiments/`，不纳入仓库。

## 已验证因素

### 1. 参数候选

从 GHCR 早期镜像形态与当前形态差异中拆出的单变量里，曾经表现较好的候选是：

- `device-ghcr-defaults`
  - `device_ram = 3.53`
  - `pid = 29418`
  - `network_radio_type = 1`
- `gpia-source-ghcr`
- `no-sim-signal`
- `client-metrics-google-play`

后续小样本显示，组合 `device-ghcr-defaults,gpia-source-ghcr,no-sim-signal` 没有比单独 `device-ghcr-defaults` 更好。当前唯一反复出现过 `sent` 的参数底座是 `device-ghcr-defaults` 单项。

### 2. 轮转出口

使用轮转出口时，结果波动较大，曾出现少量 `sent`，但大量结果仍落在 `no_routes` / `blocked`。这说明出口 IP、ASN、代理池质量或号码路由仍可能是主导因素。

### 3. 稳定出口 + `/v2/exist` 对照

稳定出口探测确认 5 次出口 hash 一致，出口地区显示为 US。随后用随机哥伦比亚号码做 `/v2/exist` 因素对照：

- `code_only`：直接发送 `/v2/code`
- `exist_then_code`：同一 fingerprint 先请求 `/v2/exist`，再请求 `/v2/code`

结果文件：

- `.temp/wa-code-param-experiments/stable-exit-exist-factor-20260617-134413.summary.json`

结果：

| 组别 | 样本数 | `sent` | `no_routes` | `blocked` | 备注 |
| --- | ---: | ---: | ---: | ---: | --- |
| `code_only` | 12 | 0 | 8 | 4 | 无 `/v2/exist` |
| `exist_then_code` | 12 | 0 | 9 | 3 | `/v2/exist` 均返回 `incorrect` |

判断：`/v2/exist` 请求本身可达，但它没有改善后续 `/v2/code`。

### 4. CO 画像一致性

尝试过把请求画像调整为哥伦比亚一致形态：

- `lg = es`
- `lc = CO`
- `mcc/mnc/sim_mcc/sim_mnc = 732/101`
- `simnum = 1`
- `sim_type = 1`

小样本结果与默认画像基本一致，没有证明它是 `no_routes` 主因。

### 5. 号码前缀

对部分哥伦比亚移动前缀做过小样本验证。`300`、`350` 都曾偶发 `sent`，但复核不稳定；`305` 也未稳定改善。因此目前不能把问题归因到某个单一号码前缀。

### 6. Python requests 与 curl/libcurl

用相同参数形态对比 Python `requests` 和 `curl/libcurl`：

- `requests`：小样本出现过 1 个 `sent`
- `curl/libcurl`：小样本未出现 `sent`

结论只能说明 curl 不是明显改善项，不能排除真实 Android WhatsApp TLS/HTTP 指纹仍是关键因素。


### 7. GPIA / WAMSYS / 设备完整性画像

后续按“`blocked` 多数是号码自身问题”的口径重算：`blocked` 只作为号码噪声记录，参数胜负主要看 `sent / (sent + no_routes)`。

完整性画像分组包括：

- `current`：当前默认画像。
- `device-ghcr`：只切换设备基础字段到早期 GHCR 形态。
- `gpia-ghcr-bundle`：只切换 GPIA 相关字段到早期 GHCR 形态。
- `wamsys-ghcr`：只切换 WAMSYS tail 到早期 GHCR 形态。
- `gpia+wamsys-ghcr`：GPIA 与 WAMSYS 同时切换。
- `device+gpia+wamsys-ghcr`：设备、GPIA、WAMSYS 同时切换。
- `full-ghcr`：完整早期 GHCR 请求形态。

第一轮小样本结果文件：

- `.temp/wa-code-param-experiments/stable-exit-integrity-profile-20260617-134922.summary.json`

按非 `blocked` 决策重算后：

| 组别 | 总样本 | `sent` | `no_routes` | `blocked` | `sent / (sent + no_routes)` |
| --- | ---: | ---: | ---: | ---: | ---: |
| `current` | 8 | 0 | 6 | 2 | 0/6 |
| `device-ghcr` | 8 | 1 | 6 | 1 | 1/7 |
| `gpia-ghcr-bundle` | 8 | 0 | 4 | 4 | 0/4 |
| `wamsys-ghcr` | 8 | 0 | 2 | 6 | 0/2 |
| `gpia+wamsys-ghcr` | 8 | 1 | 3 | 4 | 1/4 |
| `device+gpia+wamsys-ghcr` | 8 | 0 | 6 | 2 | 0/6 |
| `full-ghcr` | 8 | 0 | 5 | 3 | 0/5 |

第一轮里 `device-ghcr` 和 `gpia+wamsys-ghcr` 都出现过 `sent`，但样本太小，且 `blocked` 噪声较高。

第二轮验证了省略完整性字段是否会被协议层硬拒，结果文件：

- `.temp/wa-code-param-experiments/stable-exit-integrity-omit-20260617-135221.summary.json`

省略 `gpia/_gg/_gi`、省略 `_ga/_gp/_ge/aid`、或省略全部完整性字段，都没有出现 `bad_param` / `missing_param`。这说明这些字段不是当前代理层的硬必填校验项；但省略后也没有带来 `sent`。

第三轮按目标决策数做复核，`blocked` 不参与胜负，每个核心组尽量收集 8 个 `sent/no_routes` 决策，结果文件：

- `.temp/wa-code-param-experiments/stable-exit-integrity-target-decisions-20260617-135444.summary.json`

| 组别 | 总样本 | `sent` | `no_routes` | `blocked` | 有效决策数 | `sent / 有效决策` |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| `current` | 16 | 0 | 6 | 10 | 6 | 0/6 |
| `device-ghcr` | 12 | 0 | 8 | 4 | 8 | 0/8 |
| `gpia+wamsys-ghcr` | 16 | 0 | 8 | 8 | 8 | 0/8 |
| `device+gpia+wamsys-ghcr` | 12 | 0 | 8 | 4 | 8 | 0/8 |

复核轮没有复现第一轮的 `sent`。因此当前不能证明 GPIA / WAMSYS / 设备完整性画像调整能稳定降低 `no_routes` 或提升 `sent`。

当前判断：完整性画像仍可能有影响，但在这个稳定出口和随机 CO 号码条件下，信号不稳定；更可能被号码质量、出口国家/ASN 或真实客户端指纹噪声覆盖。


### 8. WASafe `H` 签名字段

发现 Python probe 早期一直发送 `ENC=...&H=`，这与正式服务路径不一致。

正式 Go 路径中，`/v2/exist`、`/v2/code`、`/v2/register` 都会先生成 native software attestation，然后发送：

- body：`ENC=<encrypted>&H=<signature>`
- header：`Authorization: <certificate-chain>`

无认证 envelope 才会发送纯 `ENC=<encrypted>`。`ENC=<encrypted>&H=` 属于异常形态：字段存在但签名为空。

已修复 `scripts/wa_code_param_probe.py`：

- 默认生成 signed WASafe envelope。
- body 使用非空 `H` 签名。
- header 设置 `Authorization` 证书链。
- 保留 `--unsigned` 与 `--empty-h` 仅用于对照实验。
- 输出只记录 `enc_hash` / `h_hash`，不输出原始 ENC、H、Authorization。

修复后 smoke 结果：signed envelope 能返回正常业务响应，不触发 `bad_param`。

对照结果文件：

- `.temp/wa-code-param-experiments/signed-h-vs-empty-h-20260617-141213.summary.json`

| 组别 | 总样本 | `sent` | `no_routes` | `blocked` | 有效决策数 | `sent / 有效决策` |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| `signed-h` | 11 | 0 | 8 | 3 | 8 | 0/8 |
| `empty-h-legacy` | 15 | 0 | 8 | 7 | 8 | 0/8 |

结论：`H=` 空值确实是 probe 的错误形态，脚本已修；但在当前样本下，修成 signed `H` 后仍未复现 `sent`，所以它不是单独足以解释 `no_routes` 的因素。


### 9. SMS-only 设备 UA / app version / 上下文因素

后续实验只看 SMS 方法，不再比较 voice / email 等其他 method。

第一轮非 IP 上下文因素结果文件：

- `.temp/wa-code-param-experiments/non-ip-context-factors-20260617-141902.summary.json`

结论：

- `/v2/reg_onboard_abprop` 预取可以稳定返回 `ok` 且包含 `exp_cfg`，但没有改善后续 SMS `/v2/code`。
- 稳定安装态 profile 没有改善 SMS `/v2/code`。
- 旧 app version + HUAWEI UA 触发 `bad_token`，说明 app version 与 token/服务端校验强相关，不能随便降级。

随后拆分 app version 与设备 UA：

- `.temp/wa-code-param-experiments/non-ip-ua-version-split-20260617-142148.summary.json`

| 组别 | 总样本 | `sent` | `no_routes` | `blocked` | `bad_token` | 备注 |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| `ua-current-oneplus` | 4 | 0 | 1 | 3 | 0 | 当前默认 UA |
| `ua-current-huawei` | 4 | 1 | 2 | 1 | 0 | 当前 app version + HUAWEI UA 可用 |
| `ua-old-oneplus` | 4 | 0 | 0 | 0 | 4 | 旧 app version 稳定 `bad_token` |
| `ua-old-huawei` | 4 | 0 | 0 | 0 | 4 | 旧 app version 稳定 `bad_token` |

进一步只测当前 app version 下的设备 UA：

- `.temp/wa-code-param-experiments/non-ip-current-ua-device-focus-20260617-142306.summary.json`

| 组别 | 总样本 | `sent` | `no_routes` | `blocked` | 有效决策数 | `sent / 有效决策` |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| `oneplus-current` | 18 | 0 | 6 | 9 | 6 | 0/6 |
| `huawei-current` | 16 | 3 | 5 | 5 | 8 | 3/8 |
| `samsung-current` | 12 | 1 | 7 | 3 | 8 | 1/8 |

再固定当前 app version + HUAWEI UA，只拆 SMS 参数底座：

- `.temp/wa-code-param-experiments/sms-huawei-param-split-20260617-142601.summary.json`

| 组别 | 总样本 | `sent` | `no_routes` | `blocked` | 有效决策数 | `sent / 有效决策` |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| `huawei-current` | 9 | 3 | 3 | 3 | 6 | 3/6 |
| `huawei-device-ghcr` | 7 | 3 | 3 | 1 | 6 | 3/6 |
| `huawei-gpia-wamsys` | 6 | 3 | 3 | 0 | 6 | 3/6 |
| `huawei-device-gpia-wamsys` | 7 | 3 | 3 | 1 | 6 | 3/6 |

当前判断：SMS 链路里最强的非 IP 信号是 **保持当前 app version `2.26.23.71`，但把设备 UA 从 OnePlus 切到 HUAWEI TRT-AL00A**。参数底座是否 current / device-ghcr / gpia+wamsys 对 HUAWEI UA 下的有效决策影响不明显，都是约 50% `sent`。

因此，后续如果要改服务默认画像，优先考虑当前 app version + HUAWEI 设备 UA / profile，而不是改 `/v2/exist`、IP 或 method。

## 当前排除项

- 缺少 `/v2/exist` 预热：已排除为主因。
- 单纯 CO locale / SIM / MCC 画像不一致：未看到明显影响。
- 单纯号码前缀：未看到稳定决定性影响。
- 单纯把 Python requests 换成 curl：未看到改善。
- 组合多个 GHCR 形态参数：未优于 `device-ghcr-defaults` 单项。
- Python probe 的空 `H=` 形态：已修为非空 signed `H`，但修复后仍未单独带来 `sent`。
- 旧 app version：当前 token/服务端校验下会稳定 `bad_token`，不应降级。

## 仍然可疑的因素

1. **出口质量与出口国家**
   - 稳定出口地区显示 US，而号码样本是 CO。
   - 后续应优先使用与号码国家一致、且质量稳定的出口做复验。

2. **GPIA / WAMSYS / 设备完整性画像**
   - 已做分组验证，但信号没有稳定复现，暂不能证明它是主因。
   - 当前材料仍是合成形态，可能与真实客户端特征有差异；该因素应在更稳定的同国家出口或真实客户端指纹下继续复验。

3. **设备 UA / profile 一致性**
   - SMS-only 结果显示当前 app version + HUAWEI UA 明显优于默认 OnePlus UA。
   - 需要把服务默认 profile 切到同一 HUAWEI 画像后，再用服务内 Go 客户端复验。

4. **真实客户端 TLS / HTTP 指纹**
   - Python requests 和 curl 都不等同于 Android WhatsApp 网络栈。
   - 需要用服务内 Go 客户端、真机链路或更接近 Android 的请求栈做进一步对照。

5. **设备安装态稳定性**
   - 当前探测多为每个号码生成全新 `fdid`、`expid`、`access_session_id`、`authkey`、key bundle。
   - 真实设备通常有更稳定的安装态，后续可以对比稳定 profile 多次请求与一次性 profile。

## 后续建议

- 继续以 `device-ghcr-defaults` 作为参数底座做外部变量实验。
- 不再优先验证 `/v2/exist` 是否缺失。
- 下一轮优先验证：同国家稳定出口、真实客户端/Go 客户端指纹、稳定安装态 profile。
- GPIA / WAMSYS / 设备完整性画像后续只在出口和号码质量更稳定后再复验，避免被 `blocked` 号码噪声覆盖。
- 所有实验结果继续只记录聚合状态与脱敏标识，禁止记录可复用请求材料。
- 后续 probe 默认使用 signed WASafe envelope；只有做回归对照时才使用 `--unsigned` 或 `--empty-h`。
