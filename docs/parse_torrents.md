SITE  /torrents.php

种子表格位置：`#outer > table > tbody > tr > td > table`
## 表格标题栏

#outer > table > tbody > tr > td > table > tbody > tr:nth-child(1)

## 注意事项

不同的种子的html元素格式不完全相同，差比有：
- 不一定有置顶
- 不一定有网站tag
- 不一定有促销信息 #outer > table > tbody > tr > td > table > tbody > tr:nth-child(2) > td:nth-child(2) > table > tbody > tr > td:nth-child(2) > img.pro_free

列表页字段解析规则来自站点 JSON 的 `html.torrents.fields`。KamePT 的彩色 `span[style*="background-color"]` 是官方站点标签，写入 `tag_ids`；`<br>` 后去掉官方标签、促销和进度条后的文本是列表副标题 `subtitle`，并可按空白拆分为约定俗成的 `tags`。subtitle 拆词需要配置单个 tag 最大长度，超长 token 说明不符合该约定，应过滤掉。列表页不把这段文本写入 `description`，`description` 保留给详情页简介。详情页只在用户查看详情或下载相关流程显式调用时抓取，不在站点列表 fetch 阶段自动触发。

## 一级置顶：

```
#outer > table > tbody > tr > td > table > tbody > tr:nth-child(2)
```

```HTML
<tr class="sticky-first-level">
<td class="rowfollow nowrap" valign="middle" style="padding: 0px"><a href="?cat=410"><img class="c_0" src="pic/cattrans.gif" alt="同人AV" title="同人AV" style="background-image: url(pic/category/kame/title/catsprites.svg);"></a></td>
<td class="rowfollow" width="100%" align="left" style="padding: 0px"><table class="torrentname" width="100%"><tbody><tr class="sticky-first-level"><td class="embedded" style="text-align: center;width: 46px;height: 46px"><img src="pic/cover/Getchu_4070490_4070490top.jpg" data-src="pic/cover/Getchu_4070490_4070490top.jpg" class="nexus-lazy-load preview" style="max-height: 46px;max-width: 46px"></td><td class="embedded" style="padding-left: 5px"><img class="sticky" src="pic/trans.gif" alt="Sticky" title="一级置顶">&nbsp;<img class="sticky" src="pic/trans.gif" alt="Sticky" title="一级置顶">&nbsp;<a title="[PNME-330][GETCHU-4070490][ぷにもえ！] 月明かりの郷愁 ～低身長オサナレイヤー極上フェラで手玉コロコロ口内発射＆ちっぱいぶっかけ生セックスの２発射動画～" href="details.php?id=43143&amp;hit=1"><b>[PNME-330][GETCHU-4070490][ぷにもえ！] 月明かりの郷愁 ～低身長オサナレイヤー極上フェラで手玉コロコロ口内発射＆ちっぱいぶっかけ生セックスの２発射動画～</b></a> <img class="pro_free" src="pic/trans.gif" alt="Free" onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;free&quot;&gt;免费&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-07-26 23:04:08&quot;&gt;13天21时&lt;/span&gt;&lt;/b&gt;', 'trail', false, 'delay',500,'lifetime',3000,'fade','both','styleClass','niceTitle', 'fadeMax',87, 'maxWidth', 300);"> <font color="#0000FF">剩余时间：<span title="2026-07-26 23:04:08">13天21时</span></font><br><span style="background-color:#483d8b;color:#ffffff;border-radius:0;font-size:10px;margin:0 4px 0 0;padding:1px 2px" title="">原盘</span><span style="background-color:#ff0000;color:#ffffff;border-radius:0;font-size:10px;margin:0 4px 0 0;padding:1px 2px" title="">禁转</span><span style="background-color:#8F77B5;color:#ffffff;border-radius:0;font-size:10px;margin:0 4px 0 0;padding:1px 2px" title="">自购</span><span style="background-color:#38b03f;color:#ffffff;border-radius:0;font-size:10px;margin:0 4px 0 0;padding:1px 2px" title="">新作</span><span style="background-color:#ff8c00;color:#ffffff;border-radius:0;font-size:10px;margin:0 4px 0 0;padding:1px 2px" title="">脸部无码</span>哥伦比娅 原神<div style="padding: 1px;margin-top: 2px;border: 1px solid #838383" title="seeding 100%"><div style="width: 100%;background-color: green;height: 2px"></div></div></td><td width="20" class="embedded" style="text-align: right;padding-right: 5px" valign="middle"><a href="download.php?id=43143"><img class="download" src="pic/trans.gif" style="padding-bottom: 2px;" alt="download" title="下载本种"></a><br><a id="bookmark0" href="javascript: bookmark(43143,0);"><img class="delbookmark" src="pic/trans.gif" alt="Unbookmarked" title="收藏"></a></td>
</tr></tbody></table></td><td class="rowfollow"><a href="comment.php?action=add&amp;pid=43143&amp;type=torrent" title="添加评论">0</a></td><td class="rowfollow nowrap"><span title="2026-07-11 23:04:08">1天<br>2时</span></td><td class="rowfollow">3.28<br>GB</td><td class="rowfollow" align="center"><b><a href="details.php?id=43143&amp;hit=1&amp;dllist=1#seeders">109</a></b></td>
<td class="rowfollow"><b><a href="details.php?id=43143&amp;hit=1&amp;dllist=1#leechers">1</a></b></td>
<td class="rowfollow"><a href="viewsnatches.php?id=43143"><b>169</b></a></td>
</tr>
```


分类： #outer > table > tbody > tr > td > table > tbody > tr:nth-child(2) > td:nth-child(1) > a
置顶：#outer > table > tbody > tr > td > table > tbody > tr:nth-child(2) > td:nth-child(2) > table > tbody > tr > td:nth-child(2) > img:nth-child(1)
标题：#outer > table > tbody > tr > td > table > tbody > tr:nth-child(2) > td:nth-child(2) > table > tbody > tr > td:nth-child(2) > a
网站tag：#outer > table > tbody > tr > td > table > tbody > tr:nth-child(2) > td:nth-child(2) > table > tbody > tr > td:nth-child(2) > span:nth-child(7)
等等。。。

促销信息：#outer > table > tbody > tr > td > table > tbody > tr:nth-child(3) > td:nth-child(2) > table > tbody > tr > td:nth-child(2) > img.pro_free
促销剩余时间：#outer > table > tbody > tr > td > table > tbody > tr:nth-child(3) > td:nth-child(2) > table > tbody > tr > td:nth-child(2) > font

![1783876828630](./.assets/parse_torrents/1783876828630.png)


## 二级置顶


这个有置顶，没有网站tag
```html
<tr class="sticky-second-level">
<td class="rowfollow nowrap" valign="middle" style="padding: 0px"><a href="?cat=420"><img class="c_11" src="pic/cattrans.gif" alt="外语音声" title="外语音声" style="background-image: url(pic/category/kame/title/catsprites.svg);"></a></td>
<td class="rowfollow" width="100%" align="left" style="padding: 0px"><table class="torrentname" width="100%"><tbody><tr class="sticky-second-level"><td class="embedded" style="text-align: center;width: 46px;height: 46px"><img src="pic/cover/dlsite_RJ01549068_RJ01549068_img_main.jpg" data-src="pic/cover/dlsite_RJ01549068_RJ01549068_img_main.jpg" class="nexus-lazy-load preview" style="max-height: 46px;max-width: 46px"></td><td class="embedded" style="padding-left: 5px"><img class="sticky" src="pic/trans.gif" alt="Sticky" title="二级置顶">&nbsp;<a title="[RJ01549068][くーるぼーいっす] 【純愛NTR】清楚系ボーイッシュ先輩を寝取り孕ませ妊娠確定セックス～婚約者の私を、結婚式前日に全部奪って?～&lt;バイノーラル&gt;" href="details.php?id=41107&amp;hit=1"><b>[RJ01549068][くーるぼーいっす] 【純愛NTR】清楚系ボーイッシュ先輩を寝取り孕ませ妊娠確定セックス～婚約者の私を、結婚式前日に全部奪って?～&lt;バイノーラル&gt;</b></a> <img class="pro_free" src="pic/trans.gif" alt="Free" onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;free&quot;&gt;免费&lt;/font&gt;&lt;/b&gt;剩余时间：&lt;b&gt;&lt;span title=&quot;2026-07-20 01:01:33&quot;&gt;6天23时&lt;/span&gt;&lt;/b&gt;', 'trail', false, 'delay',500,'lifetime',3000,'fade','both','styleClass','niceTitle', 'fadeMax',87, 'maxWidth', 300);"> <font color="#0000FF">剩余时间：<span title="2026-07-20 01:01:33">6天23时</span></font><br>サツキ R18 OL 浮気 快楽堕ち 純愛 退廃/背徳/インモラル 寝取り 中出し 妊娠/孕ませ 转自KCRPGX@南+</td><td width="20" class="embedded" style="text-align: right;padding-right: 5px" valign="middle"><a href="download.php?id=41107"><img class="download" src="pic/trans.gif" style="padding-bottom: 2px;" alt="download" title="下载本种"></a><br><a id="bookmark7" href="javascript: bookmark(41107,7);"><img class="delbookmark" src="pic/trans.gif" alt="Unbookmarked" title="收藏"></a></td>
</tr></tbody></table></td><td class="rowfollow"><a href="comment.php?action=add&amp;pid=41107&amp;type=torrent" title="添加评论">0</a></td><td class="rowfollow nowrap"><span title="2026-04-18 13:39:37">2月<br>25天</span></td><td class="rowfollow">3.59<br>GB</td><td class="rowfollow" align="center"><b><a href="details.php?id=41107&amp;hit=1&amp;dllist=1#seeders">4</a></b></td>
<td class="rowfollow"><b><a href="details.php?id=41107&amp;hit=1&amp;dllist=1#leechers">8</a></b></td>
<td class="rowfollow"><a href="viewsnatches.php?id=41107"><b>23</b></a></td>
</tr>
```

##  普通


```html
<tr>
<td class="rowfollow nowrap" valign="middle" style="padding: 0px"><a href="?cat=410"><img class="c_0" src="pic/cattrans.gif" alt="同人AV" title="同人AV" style="background-image: url(pic/category/kame/title/catsprites.svg);"></a></td>
<td class="rowfollow" width="100%" align="left" style="padding: 0px"><table class="torrentname" width="100%"><tbody><tr><td class="embedded" style="text-align: center;width: 46px;height: 46px"><img src="http://pic.kamept.com/?/images/2022/06/25/F0FhzRd3bI/%5BCP-103%5D%5BFC2PPV-795851.jpg" data-src="http://pic.kamept.com/?/images/2022/06/25/F0FhzRd3bI/%5BCP-103%5D%5BFC2PPV-795851.jpg" class="nexus-lazy-load preview" style="max-height: 46px;max-width: 46px"></td><td class="embedded" style="padding-left: 5px"><a title="[CP-103][FC2PPV-795851]パイパンJDレイヤーさんで遊ぼう♪冬マシュコスで電車で一人えっちしてもらいました【個人撮影】" href="details.php?id=104&amp;hit=1"><b>[CP-103][FC2PPV-795851]パイパンJDレイヤーさんで遊ぼう♪冬マシュコスで電車で一人えっちしてもらいました【個人撮影】</b></a> <b>[<font class="hot">热门</font>]</b><br>FGO	马修</td><td width="20" class="embedded" style="text-align: right;padding-right: 5px" valign="middle"><a href="download.php?id=104"><img class="download" src="pic/trans.gif" style="padding-bottom: 2px;" alt="download" title="下载本种"></a><br><a id="bookmark3" href="javascript: bookmark(104,3);"><img class="delbookmark" src="pic/trans.gif" alt="Unbookmarked" title="收藏"></a></td>
</tr></tbody></table></td><td class="rowfollow"><a href="comment.php?action=add&amp;pid=104&amp;type=torrent" title="添加评论">0</a></td><td class="rowfollow nowrap"><span title="2022-06-25 14:53:02">4年<br>1月</span></td><td class="rowfollow">404.73<br>MB</td><td class="rowfollow" align="center"><b><a href="details.php?id=104&amp;hit=1&amp;dllist=1#seeders">12</a></b></td>
<td class="rowfollow">0</td>
<td class="rowfollow"><a href="viewsnatches.php?id=104"><b>99</b></a></td>
</tr>
```

这个没有促销，也没有tag


## 详情页解析


例如上面的#outer > table > tbody > tr > td > table > tbody > tr:nth-child(2) > td:nth-child(2) > table > tbody > tr > td:nth-child(2) > a

```
<a title="[PNME-330][GETCHU-4070490][ぷにもえ！] 月明かりの郷愁 ～低身長オサナレイヤー極上フェラで手玉コロコロ口内発射＆ちっぱいぶっかけ生セックスの２発射動画～" href="details.php?id=43143&amp;hit=1"><b>[PNME-330][GETCHU-4070490][ぷにもえ！] 月明かりの郷愁 ～低身長オサナレイヤー極上フェラで手玉コロコロ口内発射＆ちっぱいぶっかけ生セックスの２発射動画～</b></a>
```

中包含  href="details.php?id=43143&amp;hit=1" 就可以得到子页面 URL：`https://kamept.com/details.php?id=43143&hit=1`

子页面的表格：
#outer > table:nth-child(4)

```html
<table width="97%" cellspacing="0" cellpadding="5">
<tbody><tr><td class="rowhead" width="13%">下载</td><td class="rowfollow" width="87%" align="left"><a class="index" href="https://kamept.com/download.php?downhash=23659.eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJpZCI6NDMxNDMsImV4cCI6MTc4MzkyMDY3N30.3rk9pwi-Xg1La2EHLHb_X2ghlJQ588vPAX6k-echQ_M">[KamePT].PNME-330哥伦比娅-原神.mp4.torrent</a>&nbsp;&nbsp;<a id="bookmark0" href="javascript: bookmark(43143,0);"><img class="delbookmark" src="pic/trans.gif" alt="Unbookmarked" title="收藏"></a>&nbsp;&nbsp;&nbsp;由&nbsp;<span class="nowrap"><a href="https://kamept.com/userdetails.php?id=14713" class="VIP_Name"><b>SakuraChan</b></a> (<span class="VIP_Name"><b>可爱桜酱</b></span>)<img src="pic/huizhang/2025_duanwu.png" title="2025 端午" class="nexus-username-medal preview" style="max-height: 11px;max-width: 11px;margin-left: 2pt"><img src="pic/huizhang/2024_zhongqiu.png" title="2024中秋" class="nexus-username-medal preview" style="max-height: 11px;max-width: 11px;margin-left: 2pt"><img src="pic/huizhang/2023zhongqiu.png" title="2023中秋" class="nexus-username-medal preview" style="max-height: 11px;max-width: 11px;margin-left: 2pt"><img src="pic/huizhang/2023_maj_first_canyu.png" title="2023第一届雀魂龟龟杯新春大奖赛参与奖" class="nexus-username-medal preview" style="max-height: 11px;max-width: 11px;margin-left: 2pt"><img src="pic/huizhang/2024_zhongyuan.png" title="2024中元" class="nexus-username-medal preview" style="max-height: 11px;max-width: 11px;margin-left: 2pt"><img src="pic/huizhang/77_0.png" title="2023七夕" class="nexus-username-medal preview" style="max-height: 11px;max-width: 11px;margin-left: 2pt"><img src="pic/huizhang/vspo_kurumi_noah_01.png" title="甲辰龙年纪念徽章 胡桃のあ" class="nexus-username-medal preview" style="max-height: 11px;max-width: 11px;margin-left: 2pt"><img src="pic/huizhang/vspo_mimi_tosaki_00.png" title="癸卯兔年 兎咲ミミ" class="nexus-username-medal preview" style="max-height: 11px;max-width: 11px;margin-left: 2pt"><img src="pic/huizhang/vspo_tsuna_nekota_01.png" title="乙巳蛇年纪念徽章 猫汰つな" class="nexus-username-medal preview" style="max-height: 11px;max-width: 11px;margin-left: 2pt"><img src="attachments/202404/20240409174243d41e8e0fb69fc64cb198114ee2a318f0.png" title="银狼点赞" class="nexus-username-medal preview" style="max-height: 11px;max-width: 11px;margin-left: 2pt"><img src="pic/huizhang/2025_zhongqiu.png" title="2025 中秋" class="nexus-username-medal preview" style="max-height: 11px;max-width: 11px;margin-left: 2pt"><img src="pic/huizhang/vspo_tsuna_nekota_02.png" title="建站三周年纪念" class="nexus-username-medal preview" style="max-height: 11px;max-width: 11px;margin-left: 2pt"><img src="pic/huizhang/quehun_lixingsai_1.png" title="第一届雀魂例行赛纪念" class="nexus-username-medal preview" style="max-height: 11px;max-width: 11px;margin-left: 2pt"><img src="pic/huizhang/星罗月兔.png" title="星罗月兔" class="nexus-username-medal preview" style="max-height: 11px;max-width: 11px;margin-left: 2pt"><img src="pic/huizhang/vspo_sendo_yuuhi_00.webp" title="vspo3D化纪念 千燈ゆうひ" class="nexus-username-medal preview" style="max-height: 11px;max-width: 11px;margin-left: 2pt"><img src="pic/huizhang/打灰爱音.png" title="打灰爱音" class="nexus-username-medal preview" style="max-height: 11px;max-width: 11px;margin-left: 2pt"><img src="pic/huizhang/vspo_yumeko_akari_00.png" title="vspo3D化纪念 夢野あかり" class="nexus-username-medal preview" style="max-height: 11px;max-width: 11px;margin-left: 2pt"><img src="pic/huizhang/aling_00.png" title="彷徨 鈴 3D化纪念" class="nexus-username-medal preview" style="max-height: 11px;max-width: 11px;margin-left: 2pt"><img src="pic/huizhang/vspo_komori_met_1.png" title="妹头四周年纪念" class="nexus-username-medal preview" style="max-height: 11px;max-width: 11px;margin-left: 2pt"><img src="pic/huizhang/2023_dongzhi.jpg" title="2023冬至" class="nexus-username-medal preview" style="max-height: 11px;max-width: 11px;margin-left: 2pt"><img src="pic/huizhang/vspo_yakumo_beni_00.png" title="vspo3D化纪念 八雲べに" class="nexus-username-medal preview" style="max-height: 11px;max-width: 11px;margin-left: 2pt"><img src="pic/huizhang/vspo_tsuna_nekota_00.png" title="vspo3D化纪念 猫汰つな" class="nexus-username-medal preview" style="max-height: 11px;max-width: 11px;margin-left: 2pt"></span>发布于<span title="2026-07-11 23:04:08">1天2时前</span></td></tr><tr><td class="rowhead nowrap" valign="top" align="right">副标题</td><td class="rowfollow" valign="top" align="left">哥伦比娅 原神</td></tr><tr><td class="rowhead nowrap" valign="top" align="right">标签</td><td class="rowfollow" valign="top" align="left"><span style="background-color:#483d8b;color:#ffffff;border-radius:0;font-size:10px;margin:0 4px 0 0;padding:1px 2px" title="">原盘</span><span style="background-color:#ff0000;color:#ffffff;border-radius:0;font-size:10px;margin:0 4px 0 0;padding:1px 2px" title="">禁转</span><span style="background-color:#8F77B5;color:#ffffff;border-radius:0;font-size:10px;margin:0 4px 0 0;padding:1px 2px" title="">自购</span><span style="background-color:#38b03f;color:#ffffff;border-radius:0;font-size:10px;margin:0 4px 0 0;padding:1px 2px" title="">新作</span><span style="background-color:#ff8c00;color:#ffffff;border-radius:0;font-size:10px;margin:0 4px 0 0;padding:1px 2px" title="">脸部无码</span></td></tr><tr><td class="rowhead nowrap" valign="top" align="right">基本信息</td><td class="rowfollow" valign="top" align="left"><b><b>大小：</b></b>3.28 GB&nbsp;&nbsp;&nbsp;<b>类型:</b>&nbsp;同人AV&nbsp;&nbsp;&nbsp;<b>马赛克: </b>脸部无码</td></tr><tr><td class="rowhead nowrap" valign="top" align="right">行为</td><td class="rowfollow" valign="top" align="left"><a title="下载种子" href="https://kamept.com/download.php?downhash=23659.eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJpZCI6NDMxNDMsImV4cCI6MTc4MzkyMDY3N30.3rk9pwi-Xg1La2EHLHb_X2ghlJQ588vPAX6k-echQ_M"><img class="dt_download" src="pic/trans.gif" alt="download">&nbsp;<b><font class="small">下载种子</font></b></a>&nbsp;|&nbsp;<a title="举报该种子违反了规则" href="report.php?torrent=43143"><img class="dt_report" src="pic/trans.gif" alt="report">&nbsp;<b><font class="small">举报种子</font></b></a></td></tr><tr><td class="rowhead nowrap" valign="top" align="right"><img src="/pic/smilies/48.gif"><span>魔法</span></td><td class="rowfollow" valign="top" align="left"><a href="promotion.php?action=view&amp;id=43143"><b>没有魔法（</b></a><br>高速咏唱 所有人（一天）：<a href="javascript:cast_magic(2, 'all')" class="faqlink">免费</a>&nbsp;/&nbsp;<a href="javascript:cast_magic(3, 'all')" class="faqlink">2X</a>&nbsp;/&nbsp;<a href="javascript:cast_magic(4, 'all')" class="faqlink">2X免费</a>&nbsp;/&nbsp;<a href="javascript:cast_magic(5, 'all')" class="faqlink">50%</a>&nbsp;/&nbsp;<a href="javascript:cast_magic(6, 'all')" class="faqlink">2X 50%</a>&nbsp;/&nbsp;<a href="javascript:cast_magic(7, 'all')" class="faqlink">30%</a><br>高速咏唱 仅自己（一天）：<a href="javascript:cast_magic(2, 'self')" class="faqlink">免费</a>&nbsp;/&nbsp;<a href="javascript:cast_magic(3, 'self')" class="faqlink">2X</a>&nbsp;/&nbsp;<a href="javascript:cast_magic(4, 'self')" class="faqlink">2X免费</a>&nbsp;/&nbsp;<a href="javascript:cast_magic(5, 'self')" class="faqlink">50%</a>&nbsp;/&nbsp;<a href="javascript:cast_magic(6, 'self')" class="faqlink">2X 50%</a>&nbsp;/&nbsp;<a href="javascript:cast_magic(7, 'self')" class="faqlink">30%</a><br><input class="btn" type="button" style="margin-top: 4px;" onclick="window.open('promotion.php?action=new&amp;id=43143')" value="施放魔法！"></td></tr><tr><td class="rowhead" valign="top">字幕</td><td class="rowfollow" align="left" valign="top"><table border="0" cellspacing="0"><tbody><tr><td class="embedded">该种子暂无字幕</td></tr></tbody></table><table border="0" cellspacing="0"><tbody><tr><td class="embedded"><form method="post" action="subtitles.php"><input type="hidden" name="torrent_name" value="[PNME-330][GETCHU-4070490][ぷにもえ！] 月明かりの郷愁 ～低身長オサナレイヤー極上フェラで手玉コロコロ口内発射＆ちっぱいぶっかけ生セックスの２発射動画～"><input type="hidden" name="detail_torrent_id" value="43143"><input type="hidden" name="in_detail" value="in_detail"><input type="submit" value="上传字幕"></form></td></tr></tbody></table></td></tr>
<tr><td class="rowhead nowrap" valign="top" align="right">商品链接</td><td class="rowfollow" valign="top" align="left"><a class="faqlink" href="https://dl.getchu.com/i/item4070490" target="_blank">https://dl.getchu.com/i/item4070490</a></td></tr><tr><td class="rowhead nowrap" valign="top" align="right"><a href="javascript: klappe_news('descr')"><span class="nowrap"><img class="minus" src="pic/trans.gif" alt="Show/Hide" id="picdescr" title="显示&nbsp;或&nbsp;隐藏"> 简介</span></a></td><td class="rowfollow" valign="top" align="left"><div id="kdescr"><div align="left" style="margin-bottom: 10px" id=""><fieldset><legend> 引用 </legend><br><font size="3"><span style="color: red;"><b>您的保种是PT站长久发展的重要保证！只要片子在硬盘上不删，请自觉做到开机即做种！自觉保种可使资源的有效期无限延长、下载速度达到极限，同时也可以为你赚得上传流量和魔力值，方便你我他快乐下载，分享至美！感谢您对KamePT的支持~</b></span></font></fieldset><br></div><img style="max-width: 100%" id="" alt="image" src="pic/cover/Getchu_4070490_4070490top.jpg" onload="Scale(this,700,0);" data-zoomable="" class="medium-zoom-image"><br>
商品名：月明かりの郷愁 ～低身長オサナレイヤー極上フェラで手玉コロコロ口内発射＆ちっぱいぶっかけ生セックスの２発射動画～<br>
番号：GETCHU-4070490<br>
商店链接：<a class="faqlink" href="https://dl.getchu.com/i/item4070490" target="_blank">https://dl.getchu.com/i/item4070490</a><br>
发售日：2026-07-12T00:00:00Z<br>
TAG：フェラ 着衣エッチ 美少女 手コキ 口内射精 美尻 美脚<br>
<img style="max-width: 100%" id="" alt="image" src="https://dl.getchu.com/data/item_img/40704/4070490/4070490_2977.jpg" onload="Scale(this,700,0);" data-zoomable="" class="medium-zoom-image" height="394" width="700"><br>
<img style="max-width: 100%" id="" alt="image" src="https://dl.getchu.com/data/item_img/40704/4070490/4070490_2978.jpg" onload="Scale(this,700,0);" data-zoomable="" class="medium-zoom-image" height="394" width="700"><br>
<img style="max-width: 100%" id="" alt="image" src="https://dl.getchu.com/data/item_img/40704/4070490/4070490_2979.jpg" onload="Scale(this,700,0);" data-zoomable="" class="medium-zoom-image" height="394" width="700"><br>
介绍：<br>
どうもこんにちは。ぷにもえです。<br>
今回は原●　コロ●ビーナコスのレイヤーさんでございます。<br>
いいですね。似合っております。白い肌に儚げな雰囲気。ビジュも含め写真で見るとめちゃくちゃキャラとの親和性が高いです。<br>
動画で見ると、普通に元気な女の子で、それはそれでキャラとのギャップでいい感じです。<br>
<br>
要するに、かわいい女の子が似合うコスプレをしている。それすなわち最高という事ですね。<br>
しかもノリノリでエロエロです。エロに前のめり系コスプレイヤーさんなので、もうね、あとは存分楽しみましょうと、そういう感じです。<br>
<br>
最初は雑談しつつ、電マで気持ちよくなってもらいます。イキそうになると止めて、またイキそうになると止めてを繰り返して焦らしつつ、感度を高めてもらいます。もう限界というところで最初の絶頂。態勢を変えつつパンツをズラして直当電マで連続絶頂を味わっていただきます。<br>
そしてパンツをズラすと見える意外としっかりとした陰毛。これは正直好みの分かれるポイントではありますが、個人的には大好きです。見た目おさなな女の子がしっかりと生えているギャップ。最高にエロいじゃないですか。イベントとかあるから剃らないのと聞くと、えーめんどいーと言っていました。そのゆるい感じもいいです。むしろ毛有は個人的にご褒美です。すみません。私の性癖はどうでもいいですね。<br>
<br>
気持ちよくなったあとは攻守交代でフェラのご奉仕タイム。<br>
様々な体勢になりながらしっかりと咥えて離さない。最後は辛抱ならんという感じでオナホ扱いのイラマで口内発射。<br>
やだーと言いながら精子を吐き出しながらもイラマは受け入れているあたりに心底では気持ちよくなっているのが見え隠れしています。<br>
一度出て落ち着いた息子を元気にさせるべく、じっくりと男を責めるターンです。<br>
キスをしながらち●こと乳首をいい塩梅で刺激してくれます。低身長レイヤーさんの乳首舐め手コキですっかりと回復したところでベッドへ移動。<br>
互いにお待ちかねの生セックスの開始です。正常位に釘打ちピストン、騎乗位からバックと体位を変えつつ何度も絶頂を繰り返していきます。<br>
最後は正常位に戻って、そのちっぱいにむけてぶっかけフィニッシュで今回の動画は以上となります。<br>
<br>
終始楽しく、そしてエロい女の子でした。<br>
それではまた。<br>
<br>
収録分数　約45分<br>
<br>
NO UPLOAD<br>
<br>
NO P2P<br>
<br>
NO mediafire etc.<br>
<br>
※本作品は分割ファイルでの配信となっております。<br>
ダウンロードページより2つのファイルをダウンロード後、<br>
4475-110221-1.part1(.exe)を実行ください。<br>
総ダウンロード容量：3.26 GB (3,504,652,703 バイト)</div></td></tr><tr><td class="rowhead nowrap" valign="top" align="right">种子文件</td><td class="rowfollow" valign="top" align="left"><table><tbody><tr><td class="no_border_wide"><b>Hash码:</b>&nbsp;786adbf1401b223785cb7f71f9fc65716b747451</td><td class="no_border_wide"><b>种子结构：</b><a href="torrent_info.php?id=43143">[查看结构]</a></td></tr></tbody></table><span id="filelist"></span></td></tr><tr><td class="rowhead nowrap" valign="top" align="right">热度表</td><td class="rowfollow" valign="top" align="left"><table><tbody><tr><td class="no_border_wide"><b>查看: </b>384</td><td class="no_border_wide"><b>点击: </b>327</td><td class="no_border_wide"><b><b>完成:</b> </b><a href="viewsnatches.php?id=43143"><b>170</b>次</a> &lt;--- 点击查看完成详情</td><td class="no_border_wide"><b>最近活动：</b><span title="2026-07-13 01:21:52">9分钟前</span></td></tr></tbody></table></td></tr><tr><td class="rowhead nowrap" valign="top" align="right">发布者带宽</td><td class="rowfollow" valign="top" align="left"><img class="speed_down" src="pic/trans.gif" alt="Downstream Rate"> 10Gbps&nbsp;&nbsp;&nbsp;&nbsp;<img class="speed_up" src="pic/trans.gif" alt="Upstream Rate"> 10Gbit&nbsp;&nbsp;&nbsp;&nbsp;Other</td></tr><tr><td class="rowhead nowrap" valign="top" align="right"><span id="seeders"></span><span id="leechers"></span>同伴<br><span id="showpeer"><a href="javascript: viewpeerlist(43143);" class="sublink">[查看列表]</a></span><span id="hidepeer" style="display: none;"><a href="javascript: hidepeerlist();" class="sublink">[隐藏列表]</a></span></td><td class="rowfollow" valign="top" align="left"><div id="peercount"><b>111个做种者</b> | <b>1个下载者</b></div><div id="peerlist"></div></td></tr><tr><td class="rowhead nowrap" valign="top" align="right">魔力值奖励</td><td class="rowfollow" valign="top" align="left"><div style="height:25px"><span id="listNumber"><input style="margin-right:5px" class="btn" type="button" onclick="saveMagicValue(43143,50);" value="+50"><input style="margin-right:5px" class="btn" type="button" onclick="saveMagicValue(43143,100);" value="+100"><input style="margin-right:5px" class="btn" type="button" onclick="saveMagicValue(43143,200);" value="+200"><input style="margin-right:5px" class="btn" type="button" onclick="saveMagicValue(43143,500);" value="+500"><input style="margin-right:5px" class="btn" type="button" onclick="saveMagicValue(43143,1000);" value="+1000"><input style="margin-right:5px" class="btn" type="button" onclick="saveMagicValue(43143,3000);" value="+3000"><input style="margin-right:5px" class="btn" type="button" onclick="saveMagicValue(43143,5000);" value="+5000"><input style="margin-right:5px" class="btn" type="button" onclick="saveMagicValue(43143,10000);" value="+10000"></span><input class="btn" type="button" id="magic_add" style="display:none" value="你已经赠送魔力值" disabled="disabled">&nbsp;目前发布人已获得<span id="spanSumAll">0</span>个魔力值奖励。&nbsp;</div><div><span id="current_user_magic" style="display:none"><span class="nowrap"><a href="https://kamept.com/userdetails.php?id=23659" class="User_Name"><b>Hammond</b></a></span></span>&nbsp;</div></td></tr><tr><td class="rowhead nowrap" valign="top" align="right">感谢者</td><td class="rowfollow" valign="top" align="left"><span id="thanksadded" style="display: none;"><input class="btn" type="button" value="感谢表示成功！" disabled="disabled"></span><span id="curuser" style="display: none;"><span class="nowrap"><a href="https://kamept.com/userdetails.php?id=23659" class="User_Name"><b>Hammond</b></a></span> </span><span id="thanksbutton"><input class="btn" type="button" id="saythanks" onclick="saythanks(43143);" value="&nbsp;&nbsp;说谢谢&nbsp;&nbsp;"></span>&nbsp;&nbsp;<span id="nothanks">暂无感谢者</span><span id="addcuruser"></span></td></tr></tbody></table>
```


需要补充给种子的内容有：
- 商品链接
- 简介
- hash
