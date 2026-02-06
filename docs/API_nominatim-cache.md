Nominatim Cache 快速使用指南

启动服务

cd server

npm install

npm run build

npm start

\# 或开发模式: npm run dev

服务运行在 http://localhost:3000

---

核心 API：逆地理编码

GET http://localhost:3000/api/reverse?lat=<纬度>\&lon=<经度>

示例：

curl "http://localhost:3000/api/reverse?lat=30.28746\&lon=120.16145"

响应：

{

&nbsp; lat: 30.2874600,

&nbsp; lon: 120.1614500,

&nbsp; display\_name: 灵隐寺, 灵隐路, 西湖区, 杭州市, 浙江省, 中国,

&nbsp; address: {

&nbsp;   tourism: temple,

&nbsp;   road: 灵隐路,

&nbsp;   city\_district: 西湖区,

&nbsp;   city: 杭州市,

&nbsp;   state: 浙江省,

&nbsp;   country: 中国,

&nbsp;   country\_code: cn

&nbsp; }

}

> 完全兼容 Nominatim 官方 API，可直接替换现有调用中的 API 地址。

---

其他应用集成

JavaScript:

const response = await fetch('http://localhost:3000/api/reverse?lat=30.28746\&lon=120.16145');

const data = await response.json();

console.log(data.display\_name);



Python:

import requests

response = requests.get('http://localhost:3000/api/reverse', params={'lat': 30.28746, 'lon': 120.16145})

data = response.json()

print(data\['display\_name'])



Node.js (axios):

const axios = require('axios');

const response = await axios.get('http://localhost:3000/api/reverse', {

&nbsp; params: { lat: 30.28746, lon: 120.16145 }

});

console.log(response.data.display\_name);

---



从官方 API 迁移

迁移前 (直接调用官方):

const response = await axios.get('https://nominatim.openstreetmap.org/reverse', {

&nbsp; params: { lat: 30.28746, lon: 120.16145, format: 'jsonv2' },

&nbsp; headers: { 'User-Agent': 'MyApp' }

});

迁移后 (使用本地缓存):

const response = await axios.get('http://localhost:3000/api/reverse', {

&nbsp; params: { lat: 30.28746, lon: 120.16145 }

});

// 返回格式完全相同，无需修改业务代码

