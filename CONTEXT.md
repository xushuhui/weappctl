# weappctl

命令行工具，代表小程序开发者在服务器端调用微信小程序服务端 API。已接入"获取已上架短剧"（`developerGetPublishedDrama`）、"获取剧目信息"（`getDrama`）、"获取剧目列表"（`listDramas`）和"删除媒资"（`deleteMedia`），后续按需追加同类接口。

## Language

**Drama（剧目）**:
通过"剧目提审"接口提交审核的一部短剧作品记录，用数字型 `drama_id` 唯一标识。`status` 字段只反映审核/下架这条生命线（审核中/审核失败/正常可播/被平台下架），跟"是否已上架"是两回事——见下面 Published Drama 的说明。
_Avoid_: Episode（指的是剧目下的单集，不是剧目本身）

**Published Drama（已上架短剧）**:
一部 Drama 被显式调用过"短剧上架设置接口"（`developerpublishdrama`）之后所处的状态；只有 Published Drama 才会出现在 `developerGetPublishedDrama` 的结果里。这是一个独立于审核状态的开关——`getDrama`/`listDramas` 返回 `status == 0`（正常可播，即审核通过）只代表这部 Drama **有资格**被上架，不代表它**已经**被上架；实测证实过这一点：某个 `status == 0` 的 Drama 能被 `getDrama` 查到，却不出现在 Published Drama 列表里，因为它没被执行过上架动作。查询 Published Drama 列表时用的是字符串型 `drama_id`（`developerGetPublishedDrama` 接口文档如此），和 `getDrama`/`listDramas` 里数字型的 `drama_id` 是两套独立编码——微信自己这几族接口文档口径不一致，weappctl 按各自接口的原始类型实现，不强行统一。
_Avoid_: Drama（不加限定时指作品记录本身，不代表已上架这一状态）

**Submitting Miniprogram（提审方小程序 / src_appid）**:
提交某个 Published Drama 上架审核的小程序，用其 `appid`（即 `src_appid`）标识。调用者查询到的 Published Drama 列表中每一条都携带自己的 Submitting Miniprogram，不必等于调用者自身的 appid。
_Avoid_: Owner appid, App ID（不带限定容易和调用方自己的 appid 混淆）

**Access Token**:
调用微信服务端 API 的凭证字符串。本项目通过 appid + secret 向微信换取，有效期 7200 秒，过期前需要重新换取；weappctl 只支持这一种换取方式，不支持第三方代商家场景使用的 `authorizer_access_token`。
_Avoid_: Token（在本项目里 token 专指 access token，不作为通用词使用）

**Profile**:
weappctl 配置文件中的一组具名凭证（appid + secret），对应一个小程序账号。用户可以配置多个 Profile 并通过名称切换，每个 Profile 各自维护自己的 access token 缓存。
_Avoid_: Account, Credential set

**Media（媒资）**:
独立于 Drama 存在的一份已上传音视频/图片资产，用数字型 `media_id` 唯一标识，通过 `deleteMedia` 等媒资管理接口操作。Drama 的 `media_list` 字段（见 `DramaMedia`）只是对媒资的引用，不是媒资本身——删除一份 Media 不等于删除某个 Drama 或其条目；反过来，一份仍被某集占用的 Media 若被删除，微信侧会以错误码拒绝（如 10090040 剧集已经被占用）。
_Avoid_: Asset（不加限定时含义太泛，本项目里专指微信媒资管理接口操作的对象）

**Audit Status（审核状态）**:
一部 Drama 在"剧目提审"这条生命线里所处的位置，取自 `audit_detail.status`，weappctl 对外统一用这几个词表达：`invalid`（0 无效）、`in-review`（1 审核中）、`rejected`（2 终审不通过）、`approved`（3 审核通过）、`returned`（4 退回待修改）。这是**权威**的审核信号——`DramaInfo.status` 里的 `1` 并不可靠（实测 167 条中只有 2 条真在审核中，其余 165 条实际是"退回修改"），CLI 与 MCP 工具一律以 Audit Status 为准。
_Avoid_: Status（不加限定时会和 `DramaInfo.status` 那个粗粒度可播标记混淆）

**Taken Down（被平台下架）**:
一部 Drama 被平台下架的状态，取自 `DramaInfo.status == 3`。它与 Audit Status 是两个正交维度：一部 Drama 可以同时 `approved` 且 `taken_down`（实测 23 条）。实测还确认了 `DramaInfo.status` 的四个取值可由 Audit Status + Taken Down 完全推得，所以 weappctl 不再把那两个原始整数作为主要输出，只在显式索取时原样给出。
_Avoid_: 下架（口语里容易和"审核不通过"混为一谈）
