package task

import "testing"

func TestSplitKeyEventsKeepsPunctuationOnOneLine(t *testing.T) {
	events := SplitKeyEvents("  雨夜相遇，并拆开信件  \n\n \n灯还亮着\r")
	if len(events) != 2 || events[0] != "雨夜相遇，并拆开信件" || events[1] != "灯还亮着" {
		t.Fatalf("events = %#v", events)
	}
	if len(SplitKeyEvents(" \n\t")) != 0 {
		t.Fatal("blank key events should be an empty list")
	}
}

func TestScoreReviewChecklistSeparatesJudgmentFromQuotation(t *testing.T) {
	const body = "他在雨夜里推开了门。灯还亮着。"
	const events = "雨夜进门\n\n灯还亮着\n缺了一字"
	raw := "" +
		"关键事件：雨夜进门\n判断：已完成\n引文：他在雨夜里推开了门\n位置：开头\n" +
		"关键事件：灯还亮着\n判断：未完成\n引文：灯还亮着\n位置：后一句\n" +
		"关键事件：缺了一字\n判断：通过\n引文：他在雨夜推开了门\n位置：无\n" +
		"关键事件：清单以外的事\n判断：已完成\n引文：他在雨夜里推开了门\n位置：开头\n"
	findings := ScoreReviewChecklist("chapter-1", body, events, raw)
	if len(findings) != 3 {
		t.Fatalf("findings = %#v", findings)
	}
	done := findings[0]
	if done.Status != "已完成" || done.ModelJudgment != "已完成" || !done.QuoteVerbatim || done.Quote != "他在雨夜里推开了门" || done.Location != "开头" {
		t.Fatalf("verbatim completion = %#v", done)
	}
	kept := findings[1]
	if kept.Status != "未完成" || kept.ModelJudgment != "未完成" || !kept.QuoteVerbatim {
		t.Fatalf("verbatim quote must not become 已完成: %#v", kept)
	}
	fuzzy := findings[2]
	if fuzzy.Status != "待核实" || fuzzy.ModelJudgment != "已完成" || fuzzy.QuoteVerbatim {
		t.Fatalf("通过 with a missing character = %#v", fuzzy)
	}
}

func TestScoreReviewChecklistMarksOmittedEventsUnfinished(t *testing.T) {
	findings := ScoreReviewChecklist("chapter-1", "他在雨夜里推开了门。", "写到了进门\n没写的离别", "关键事件：写到了进门\n判断：已完成\n引文：他在雨夜里推开了门\n位置：全文\n")
	if len(findings) != 2 || findings[0].Status != "已完成" || findings[1].Status != "未完成" || findings[1].ModelJudgment != "" || findings[1].QuoteVerbatim {
		t.Fatalf("omitted event = %#v", findings)
	}
}

func TestNormalizeReviewJudgmentDoesNotTreatFailureAsPass(t *testing.T) {
	if judgment, ok := NormalizeReviewJudgment("不通过"); !ok || judgment != "未完成" {
		t.Fatalf("不通过 = %q %v", judgment, ok)
	}
	if judgment, ok := NormalizeReviewJudgment("通过"); !ok || judgment != "已完成" {
		t.Fatalf("通过 = %q %v", judgment, ok)
	}
	if QuoteIsVerbatim("灯还亮着", "灯还亮着。") {
		t.Fatal("a longer quotation is not a substring")
	}
}
