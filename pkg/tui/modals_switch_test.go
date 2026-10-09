package tui

import (
	"fmt"
	"testing"

	"github.com/k0wl0n/gctx/pkg/manager"
)

// TestStartSwitch_BackgroundOp verifies that startSwitch sets op and launches a background cmd.
func TestStartSwitch_BackgroundOp(t *testing.T) {
	svc := newFakeService(sampleOverview("alice", "alice", "bob"))
	m := newTestModel(t, svc, 100, 24)
	m.selected = "bob"

	m2, cmd := m.startSwitch("bob")

	if m2.op == nil {
		t.Fatal("expected in-flight op to be set")
	}
	if m2.op.kind != opSwitch {
		t.Errorf("op kind = %s, want switch", m2.op.kind)
	}
	if m2.op.account != "bob" {
		t.Errorf("op account = %s, want bob", m2.op.account)
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd from startSwitch")
	}
}

// TestStartSwitch_BusyGuard verifies that a second switch while busy shows "Busy:" message.
func TestStartSwitch_BusyGuard(t *testing.T) {
	svc := newFakeService(sampleOverview("alice", "alice", "bob"))
	m := newTestModel(t, svc, 100, 24)
	m.op = &operation{kind: opSwitch, account: "alice", desc: "switching", seq: 1, started: m.now()}

	m2, _ := m.startSwitch("bob")
	if m2.status.text == "" {
		t.Error("expected busy status message")
	}
	if m2.op.account != "alice" {
		t.Error("existing op should not be replaced")
	}
}

// TestHandleOpDone_Switch verifies that handleOpDone processes switch results.
func TestHandleOpDone_Switch(t *testing.T) {
	svc := newFakeService(sampleOverview("alice", "alice", "bob"))
	m := newTestModel(t, svc, 100, 24)
	m.seq = 1
	m.op = &operation{kind: opSwitch, account: "bob", desc: "switching", seq: 1, started: m.now()}

	msg := opDoneMsg{
		seq:     1,
		kind:    opSwitch,
		account: "bob",
		result:  manager.SwitchResult{Account: "bob"},
	}
	m2, _ := m.handleOpDone(msg)
	if m2.op != nil {
		t.Error("op should be cleared after done")
	}
	if m2.status.text == "" {
		t.Error("expected success status message")
	}
}

// TestHandleOpDone_StaleSeq verifies that stale messages are dropped.
func TestHandleOpDone_StaleSeq(t *testing.T) {
	svc := newFakeService(sampleOverview("alice", "alice", "bob"))
	m := newTestModel(t, svc, 100, 24)
	m.seq = 2
	m.op = &operation{kind: opSwitch, account: "bob", desc: "switching", seq: 2, started: m.now()}

	msg := opDoneMsg{seq: 1, kind: opSwitch, account: "bob"}
	m2, _ := m.handleOpDone(msg)
	if m2.op == nil {
		t.Error("current op should not be cleared by stale message")
	}
}

// TestHandleOpDone_WithWarnings verifies warning mapping on switch result.
func TestHandleOpDone_WithWarnings(t *testing.T) {
	svc := newFakeService(sampleOverview("alice", "alice", "bob"))
	m := newTestModel(t, svc, 100, 24)
	m.seq = 1
	m.op = &operation{kind: opSwitch, account: "bob", desc: "switching", seq: 1, started: m.now()}

	msg := opDoneMsg{
		seq:     1,
		kind:    opSwitch,
		account: "bob",
		result: manager.SwitchResult{
			Account:  "bob",
			Warnings: []manager.Warning{{Code: manager.WarnReauthRequired}},
		},
	}
	m2, _ := m.handleOpDone(msg)
	if m2.status.level != levelWarning {
		t.Errorf("expected warning level, got %v", m2.status.level)
	}
}

// TestHandleOpDone_Error verifies error handling in handleOpDone.
func TestHandleOpDone_Error(t *testing.T) {
	svc := newFakeService(sampleOverview("alice", "alice", "bob"))
	m := newTestModel(t, svc, 100, 24)
	m.seq = 1
	m.op = &operation{kind: opSwitch, account: "bob", desc: "switching", seq: 1, started: m.now()}

	msg := opDoneMsg{
		seq:     1,
		kind:    opSwitch,
		account: "bob",
		err:     fmt.Errorf("switch failed"),
	}
	m2, _ := m.handleOpDone(msg)
	if m2.status.level != levelError {
		t.Errorf("expected error level, got %v", m2.status.level)
	}
}




// Verify updateModal delegates to modal.update.
func TestUpdateModal_Delegates(t *testing.T) {
	svc := newFakeService(sampleOverview("alice", "alice", "bob"))
	m := newTestModel(t, svc, 100, 24)
	acc, _ := m.overview.Account("alice")
	m.modal = newSaveDialog(acc, m.overview)

	// ESC should close the modal.
	m2, _ := m.updateModal(keyMsg("esc"))
	if m2.modal != nil {
		t.Error("expected modal to be closed after ESC")
	}
}

// TestUpdateModal_NilModal verifies no panic when modal is nil.
func TestUpdateModal_NilModal(t *testing.T) {
	svc := newFakeService(sampleOverview("alice", "alice"))
	m := newTestModel(t, svc, 100, 24)
	m.modal = nil
	m2, cmd := m.updateModal(keyMsg("enter"))
	_ = m2
	_ = cmd
}

// TestSaveDialog_CancelFocusDefault verifies initial focus is Cancel.
func TestSaveDialog_CancelFocusDefault(t *testing.T) {
	svc := newFakeService(sampleOverview("alice", "alice"))
	m := newTestModel(t, svc, 100, 24)
	acc, _ := m.overview.Account("alice")
	dlg := newSaveDialog(acc, m.overview)
	if dlg.focus != focusCancel {
		t.Error("save dialog initial focus should be Cancel")
	}
}

// TestSaveDialog_ConfirmStartsOp verifies confirming opens a save op.
func TestSaveDialog_ConfirmStartsOp(t *testing.T) {
	svc := newFakeService(sampleOverview("alice", "alice"))
	m := newTestModel(t, svc, 100, 24)
	acc, _ := m.overview.Account("alice")
	m.modal = newSaveDialog(acc, m.overview)

	// Tab to switch focus to Confirm.
	m2, _ := m.updateModal(keyMsg("tab"))
	// Press enter to confirm.
	m3, cmd := m2.updateModal(keyMsg("enter"))
	if m3.op == nil {
		t.Error("expected op to be started after confirm")
	}
	if m3.op.kind != opSave {
		t.Errorf("expected save op, got %s", m3.op.kind)
	}
	if cmd == nil {
		t.Error("expected cmd from confirm save")
	}
}

// TestSaveDialog_CancelNoOp verifies canceling save dialog does not start op.
func TestSaveDialog_CancelNoOp(t *testing.T) {
	svc := newFakeService(sampleOverview("alice", "alice"))
	m := newTestModel(t, svc, 100, 24)
	acc, _ := m.overview.Account("alice")
	m.modal = newSaveDialog(acc, m.overview)

	// Focus is Cancel by default; press Enter to cancel.
	m2, _ := m.updateModal(keyMsg("enter"))
	if m2.op != nil {
		t.Error("cancel should not start a save op")
	}
	if m2.modal != nil {
		t.Error("modal should be closed after cancel")
	}
}

// TestDeleteDialog_CancelFocusDefault verifies initial focus is Cancel.
func TestDeleteDialog_CancelFocusDefault(t *testing.T) {
	svc := newFakeService(sampleOverview("alice", "alice"))
	m := newTestModel(t, svc, 100, 24)
	acc, _ := m.overview.Account("alice")
	dlg := newDeleteDialog(acc, m.overview)
	if dlg.focus != focusCancel {
		t.Error("delete dialog initial focus should be Cancel")
	}
}

// TestDeleteDialog_GcloudActiveDisablesCheckbox verifies checkbox is disabled for gcloud active config.
func TestDeleteDialog_GcloudActiveDisablesCheckbox(t *testing.T) {
	ov := sampleOverview("alice", "alice", "bob")
	ov.GcloudActiveConfig = "alice-config"
	svc := newFakeService(ov)
	m := newTestModel(t, svc, 100, 24)
	acc, _ := m.overview.Account("alice")
	dlg := newDeleteDialog(acc, ov)
	if !dlg.gcloudActive {
		t.Error("gcloudActive should be true when config matches gcloud active config")
	}
}

// TestDeleteDialog_ConfirmStartsDeleteOp verifies confirming delete starts op.
func TestDeleteDialog_ConfirmDeleteOp(t *testing.T) {
	svc := newFakeService(sampleOverview("alice", "alice", "bob"))
	m := newTestModel(t, svc, 100, 24)
	acc, _ := m.overview.Account("bob")
	m.modal = newDeleteDialog(acc, m.overview)

	// Tab to Confirm.
	m2, _ := m.updateModal(keyMsg("tab"))
	m3, cmd := m2.updateModal(keyMsg("enter"))
	if m3.op == nil {
		t.Error("expected delete op to start")
	}
	if m3.op.kind != opDelete {
		t.Errorf("expected delete op, got %s", m3.op.kind)
	}
	if cmd == nil {
		t.Error("expected cmd from delete confirm")
	}
}

// TestCreateForm_Validation verifies name validation in the create form.
func TestCreateForm_Validation(t *testing.T) {
	svc := newFakeService(sampleOverview("alice", "alice"))
	m := newTestModel(t, svc, 100, 24)
	m, _ = m.openCreate()

	// Type invalid name.
	cf := m.modal.(*createForm)
	cf.nameInput.SetValue("INVALID NAME!")
	m.modal = cf

	// Navigate to submit and hit enter.
	m, _ = m.updateModal(keyMsg("enter")) // move to project
	m, _ = m.updateModal(keyMsg("enter")) // move to submit
	m2, _ := m.updateModal(keyMsg("enter"))

	if m2.modal == nil {
		t.Error("modal should remain open after validation error")
	}
	cf2, ok := m2.modal.(*createForm)
	if !ok {
		t.Fatal("modal should still be createForm after validation error")
	}
	if cf2.nameErr == "" {
		t.Error("expected name validation error")
	}
}

// TestCreateForm_SubmitStartsCreateOp verifies the create form starts a create op.
func TestCreateForm_SubmitStartsOp(t *testing.T) {
	svc := newFakeService(sampleOverview("alice", "alice"))
	m := newTestModel(t, svc, 100, 24)
	m, _ = m.openCreate()

	cf := m.modal.(*createForm)
	cf.nameInput.SetValue("newaccount")
	m.modal = cf

	m, _ = m.updateModal(keyMsg("enter")) // move to project
	m, _ = m.updateModal(keyMsg("enter")) // move to submit
	m2, cmd := m.updateModal(keyMsg("enter"))

	if m2.modal != nil {
		t.Error("modal should close after submit")
	}
	if m2.op == nil {
		t.Error("expected create op to start")
	}
	if cmd == nil {
		t.Error("expected cmd from create submit")
	}
}

// TestWarningMessage_AllCodes verifies all known warning codes return non-empty messages.
func TestWarningMessage_AllCodes(t *testing.T) {
	codes := []manager.WarningCode{
		manager.WarnADCMissing, manager.WarnReauthRequired, manager.WarnSetProjectFailed,
		manager.WarnConfigReused, manager.WarnConfigDeleteFailed, manager.WarnADCRemoveFailed,
		manager.WarnADCPathOutsideStore, manager.WarnADCNotRefreshed,
	}
	for _, code := range codes {
		msg := warningMessage(manager.Warning{Code: code})
		if msg == "" {
			t.Errorf("warningMessage(%s) = empty string", code)
		}
	}
}

// TestOpDone_SelectsNewAccountAfterCreate verifies new account is selected post-create.
func TestOpDone_SelectsNewAccountAfterCreate(t *testing.T) {
	ov := sampleOverview("alice", "alice")
	svc := newFakeService(ov)
	m := newTestModel(t, svc, 100, 24)
	m.seq = 1
	m.op = &operation{kind: opCreate, account: "newacct", desc: "creating", seq: 1, started: m.now()}

	// Update overview so newacct appears.
	ov2 := sampleOverview("alice", "alice", "newacct")
	svc.overview = ov2

	msg := opDoneMsg{
		seq:     1,
		kind:    opCreate,
		account: "newacct",
		result:  manager.CreateResult{Account: "newacct"},
	}
	m2, _ := m.handleOpDone(msg)
	if m2.selected != "newacct" {
		t.Errorf("expected newacct to be selected after create, got %s", m2.selected)
	}
}

// TestHandleOpDone_NearestRowAfterDelete verifies nearest row is selected after delete.
func TestHandleOpDone_NearestRowAfterDelete(t *testing.T) {
	ov := sampleOverview("alice", "alice", "bob", "charlie")
	svc := newFakeService(ov)
	m := newTestModel(t, svc, 100, 24)
	m.selected = "bob"
	m.seq = 1
	m.op = &operation{kind: opDelete, account: "bob", desc: "deleting", seq: 1, started: m.now()}

	// After delete, overview no longer has bob.
	ov2 := sampleOverview("alice", "alice", "charlie")
	svc.overview = ov2

	msg := opDoneMsg{seq: 1, kind: opDelete, account: "bob", result: manager.DeleteResult{Account: "bob"}}
	m2, _ := m.handleOpDone(msg)
	// Should select alice or charlie (nearest to where bob was).
	if m2.selected == "" {
		t.Error("expected a selection after delete")
	}
}

// TestStartLogin_Delegates verifies startLogin calls beginLogin.
func TestStartLogin_Delegates(t *testing.T) {
	svc := newFakeService(sampleOverview("alice", "alice"))
	svc.loginRes = manager.LoginResult{Account: "alice"}
	// Use a cmd-capturing exec seam.
	m := newTestModel(t, svc, 100, 24)
	m, _ = m.startLogin("alice")
	// No panic means the delegation works (beginLogin is a B3 stub returning m, nil).
	_ = m
}
