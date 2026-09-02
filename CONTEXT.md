# weappctl

命令行工具，代表小程序开发者在服务器端调用微信小程序服务端 API。已接入"获取已上架短剧"（`developerGetPublishedDrama`）、"获取剧目信息"（`getDrama`）和"获取剧目列表"（`listDramas`），后续按需追加同类接口。

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
