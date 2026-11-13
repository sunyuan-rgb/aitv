// 短剧内容上架、付费观看与下架处理 —— 题目04 服务入口。
package main

import (
	"flag"
	"log"

	"aitv/internal/billing"
	"aitv/internal/content"
	"aitv/internal/refund"
	"aitv/internal/sales"
	"aitv/internal/store"
	"aitv/internal/web"
)

func main() {
	addr := flag.String("addr", ":8080", "监听地址")
	dbPath := flag.String("db", "data/aitv.db", "SQLite 数据库文件路径")
	flag.Parse()

	st, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("打开数据库失败: %v", err)
	}
	if err := st.Seed(); err != nil {
		log.Fatalf("预置数据失败: %v", err)
	}

	contentSvc := content.New(st)
	salesSvc := sales.New(st, contentSvc)
	billingSvc := billing.New(st)
	refundSvc := refund.New(st, billingSvc)

	srv := &web.Server{
		St:       st,
		Sessions: web.NewSessions(),
		Content:  contentSvc,
		Sales:    salesSvc,
		Billing:  billingSvc,
		Refund:   refundSvc,
	}

	log.Printf("短剧平台服务启动于 %s（数据库 %s）", *addr, *dbPath)
	if err := srv.Router().Run(*addr); err != nil {
		log.Fatal(err)
	}
}
