<script setup lang="ts">
import { NButton, NCard } from 'naive-ui'
</script>

<template>
  <main class="home-shell">
    <section class="hero-card">
      <div class="eyebrow">Epay payment platform</div>
      <h1>用户自助接入支付产品</h1>
      <p>
        注册用户账号后，可以创建产品、获取 PID/PKEY、查看订单、管理回调地址并申请提现。
        管理员后台仍保留独立入口，用于管理用户、产品、订单、提现和系统配置。
      </p>
      <div class="actions">
        <router-link to="/user/register" custom v-slot="{ href, navigate }">
          <n-button tag="a" :href="href" type="primary" size="large" @click="navigate">用户注册</n-button>
        </router-link>
        <router-link to="/user/login" custom v-slot="{ href, navigate }">
          <n-button tag="a" :href="href" secondary size="large" @click="navigate">用户登录</n-button>
        </router-link>
        <router-link to="/admin/login" custom v-slot="{ href, navigate }">
          <n-button tag="a" :href="href" tertiary size="large" @click="navigate">管理员登录</n-button>
        </router-link>
      </div>
    </section>

    <section class="cards">
      <n-card title="1. 注册用户">
        用邮箱注册用户账号；用户是收款、结算和提现主体。
      </n-card>
      <n-card title="2. 创建产品">
        每个产品拥有独立 PID/PKEY，可配置 notify_url、return_url 和可选费率。
      </n-card>
      <n-card title="3. 接入 Demo">
        使用产品 PID/PKEY 调用独立 Demo 或 EasyPay /mapi.php 接口完成支付测试。
      </n-card>
    </section>
  </main>
</template>

<style scoped>
.home-shell {
  min-height: 100vh;
  display: grid;
  align-content: center;
  gap: 24px;
  padding: 48px 24px;
  background: radial-gradient(circle at 16% 10%, rgba(83, 58, 253, 0.16), transparent 34%), #f6f9fc;
}
.hero-card,
.cards {
  width: min(1040px, 100%);
  margin: 0 auto;
}
.hero-card {
  padding: clamp(28px, 5vw, 54px);
  border: 1px solid #e3e8ee;
  border-radius: 24px;
  background: #fff;
  box-shadow: 0 18px 50px -24px rgba(13, 37, 61, 0.28);
}
.eyebrow {
  display: inline-flex;
  padding: 6px 12px;
  border-radius: 999px;
  background: rgba(83, 58, 253, 0.1);
  color: #4434d4;
  font-size: 13px;
  font-weight: 800;
}
h1 {
  margin: 18px 0 10px;
  color: #0d253d;
  font-size: clamp(34px, 5vw, 56px);
  line-height: 1.06;
  letter-spacing: -0.045em;
}
p {
  max-width: 760px;
  margin: 0;
  color: #64748d;
  font-size: 16px;
  line-height: 1.8;
}
.actions {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
  margin-top: 28px;
}
.actions a {
  text-decoration: none;
}
.cards {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 16px;
}
@media (max-width: 820px) {
  .cards {
    grid-template-columns: 1fr;
  }
}
</style>
