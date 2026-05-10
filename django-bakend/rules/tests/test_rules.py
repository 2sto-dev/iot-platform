"""Tests for Faza 4.1 — Rule engine."""
import pytest
from django.contrib.auth import get_user_model
from rest_framework.test import APIClient

from rules.models import Rule, RuleExecution
from rules.validators import validate_condition_node, validate_actions
from tenants.models import Membership, Tenant
from rest_framework.exceptions import ValidationError

User = get_user_model()

SIMPLE_CONDITION = {"field": "power_w", "op": "gt", "value": 1000}
NESTED_CONDITION = {
    "operator": "AND",
    "conditions": [
        {"field": "power_w", "op": "gt", "value": 1000},
        {"operator": "OR", "conditions": [
            {"field": "temperature", "op": "gte", "value": 80},
            {"field": "error", "op": "is_not_null"},
        ]},
    ],
}
SIMPLE_ACTIONS = [{"type": "downlink", "action": "alert_led", "payload": {"color": "red"}}]


@pytest.fixture
def api():
    return APIClient()


@pytest.fixture
def tenant(db):
    return Tenant.objects.create(name="RuleCo", slug="ruleco")


@pytest.fixture
def other_tenant(db):
    return Tenant.objects.create(name="Other", slug="other-rules")


def _make_user(username, tenant, role):
    user = User.objects.create_user(username=username, password="pw", prenume=username.title())
    Membership.objects.create(user=user, tenant=tenant, role=role)
    return user


@pytest.fixture
def owner(db, tenant):
    return _make_user("rule_owner", tenant, Membership.Role.OWNER)


@pytest.fixture
def viewer(db, tenant):
    return _make_user("rule_viewer", tenant, Membership.Role.VIEWER)


def _jwt(user, tenant):
    from rest_framework_simplejwt.tokens import AccessToken
    token = AccessToken.for_user(user)
    token["tenant_id"] = tenant.id
    token["role"] = Membership.objects.filter(user=user, tenant=tenant).first().role
    return str(token)


# ── DSL validator ─────────────────────────────────────────────────────────────

class TestConditionValidator:
    def test_valid_leaf(self):
        validate_condition_node({"field": "power", "op": "gt", "value": 100})

    def test_valid_and(self):
        validate_condition_node(NESTED_CONDITION)

    def test_valid_not(self):
        validate_condition_node({"operator": "NOT", "condition": {"field": "x", "op": "is_null"}})

    def test_invalid_op(self):
        with pytest.raises(ValidationError):
            validate_condition_node({"field": "x", "op": "unknown"})

    def test_missing_value(self):
        with pytest.raises(ValidationError):
            validate_condition_node({"field": "x", "op": "gt"})

    def test_in_requires_list(self):
        with pytest.raises(ValidationError):
            validate_condition_node({"field": "x", "op": "in", "value": "not-a-list"})

    def test_empty_and(self):
        with pytest.raises(ValidationError):
            validate_condition_node({"operator": "AND", "conditions": []})

    def test_both_operator_and_field(self):
        with pytest.raises(ValidationError):
            validate_condition_node({"operator": "AND", "field": "x", "conditions": []})

    def test_no_value_for_is_null(self):
        validate_condition_node({"field": "x", "op": "is_null"})

    def test_changed_no_value_required(self):
        validate_condition_node({"field": "relay_state", "op": "changed"})


class TestActionValidator:
    def test_valid_downlink(self):
        validate_actions([{"type": "downlink", "action": "restart"}])

    def test_valid_notify(self):
        validate_actions([{"type": "notify", "channel_id": 1, "body": "Alert!"}])

    def test_valid_webhook(self):
        validate_actions([{"type": "webhook", "url": "https://x.com/hook"}])

    def test_valid_set_shadow(self):
        validate_actions([{"type": "set_shadow", "desired": {"relay": "off"}}])

    def test_invalid_type(self):
        with pytest.raises(ValidationError):
            validate_actions([{"type": "magic"}])

    def test_empty_list(self):
        with pytest.raises(ValidationError):
            validate_actions([])

    def test_downlink_missing_action(self):
        with pytest.raises(ValidationError):
            validate_actions([{"type": "downlink"}])


# ── API endpoint tests ────────────────────────────────────────────────────────

@pytest.mark.django_db
class TestRuleAPI:
    def test_owner_can_create_rule(self, api, owner, tenant):
        token = _jwt(owner, tenant)
        api.credentials(HTTP_AUTHORIZATION=f"Bearer {token}")
        resp = api.post("/api/v1/rules/", {
            "name": "High power alert",
            "trigger_stream_pattern": "telemetry",
            "conditions": SIMPLE_CONDITION,
            "actions": SIMPLE_ACTIONS,
            "cooldown_seconds": 120,
        }, format="json")
        assert resp.status_code == 201
        assert resp.data["name"] == "High power alert"
        assert resp.data["enabled"] is True

    def test_viewer_cannot_create_rule(self, api, viewer, tenant):
        token = _jwt(viewer, tenant)
        api.credentials(HTTP_AUTHORIZATION=f"Bearer {token}")
        resp = api.post("/api/v1/rules/", {
            "name": "x", "conditions": SIMPLE_CONDITION,
            "actions": SIMPLE_ACTIONS,
        }, format="json")
        assert resp.status_code == 403

    def test_owner_can_list_rules(self, api, owner, tenant):
        Rule.objects.create(
            tenant=tenant, name="r1",
            conditions=SIMPLE_CONDITION, actions=SIMPLE_ACTIONS,
        )
        token = _jwt(owner, tenant)
        api.credentials(HTTP_AUTHORIZATION=f"Bearer {token}")
        resp = api.get("/api/v1/rules/")
        assert resp.status_code == 200
        assert len(resp.data) >= 1

    def test_tenant_isolation(self, api, owner, tenant, other_tenant, db):
        Rule.objects.create(
            tenant=other_tenant, name="other-rule",
            conditions=SIMPLE_CONDITION, actions=SIMPLE_ACTIONS,
        )
        token = _jwt(owner, tenant)
        api.credentials(HTTP_AUTHORIZATION=f"Bearer {token}")
        resp = api.get("/api/v1/rules/")
        assert resp.status_code == 200
        names = [r["name"] for r in resp.data]
        assert "other-rule" not in names

    def test_toggle_rule(self, api, owner, tenant):
        rule = Rule.objects.create(
            tenant=tenant, name="toggle-me",
            conditions=SIMPLE_CONDITION, actions=SIMPLE_ACTIONS, enabled=True,
        )
        token = _jwt(owner, tenant)
        api.credentials(HTTP_AUTHORIZATION=f"Bearer {token}")
        resp = api.patch(f"/api/v1/rules/{rule.id}/toggle/")
        assert resp.status_code == 200
        assert resp.data["enabled"] is False
        resp2 = api.patch(f"/api/v1/rules/{rule.id}/toggle/")
        assert resp2.data["enabled"] is True

    def test_invalid_condition_rejected(self, api, owner, tenant):
        token = _jwt(owner, tenant)
        api.credentials(HTTP_AUTHORIZATION=f"Bearer {token}")
        resp = api.post("/api/v1/rules/", {
            "name": "bad",
            "conditions": {"field": "x", "op": "unknown_op"},
            "actions": SIMPLE_ACTIONS,
        }, format="json")
        assert resp.status_code == 400

    def test_nested_and_or_condition(self, api, owner, tenant):
        token = _jwt(owner, tenant)
        api.credentials(HTTP_AUTHORIZATION=f"Bearer {token}")
        resp = api.post("/api/v1/rules/", {
            "name": "complex",
            "conditions": NESTED_CONDITION,
            "actions": SIMPLE_ACTIONS,
        }, format="json")
        assert resp.status_code == 201

    def test_delete_rule(self, api, owner, tenant):
        rule = Rule.objects.create(
            tenant=tenant, name="to-delete",
            conditions=SIMPLE_CONDITION, actions=SIMPLE_ACTIONS,
        )
        token = _jwt(owner, tenant)
        api.credentials(HTTP_AUTHORIZATION=f"Bearer {token}")
        resp = api.delete(f"/api/v1/rules/{rule.id}/")
        assert resp.status_code == 204
        assert not Rule.objects.filter(pk=rule.id).exists()

    def test_execution_history(self, api, owner, tenant):
        rule = Rule.objects.create(
            tenant=tenant, name="exec-rule",
            conditions=SIMPLE_CONDITION, actions=SIMPLE_ACTIONS,
        )
        RuleExecution.objects.create(
            rule=rule, rule_name=rule.name, tenant=tenant,
            device_serial="DEV001", stream="telemetry",
            status=RuleExecution.Status.TRIGGERED,
        )
        token = _jwt(owner, tenant)
        api.credentials(HTTP_AUTHORIZATION=f"Bearer {token}")
        resp = api.get(f"/api/v1/rules/{rule.id}/executions/")
        assert resp.status_code == 200
        assert len(resp.data) == 1
        assert resp.data[0]["device_serial"] == "DEV001"

    def test_duplicate_name_rejected(self, api, owner, tenant):
        Rule.objects.create(
            tenant=tenant, name="dup",
            conditions=SIMPLE_CONDITION, actions=SIMPLE_ACTIONS,
        )
        token = _jwt(owner, tenant)
        api.credentials(HTTP_AUTHORIZATION=f"Bearer {token}")
        resp = api.post("/api/v1/rules/", {
            "name": "dup", "conditions": SIMPLE_CONDITION, "actions": SIMPLE_ACTIONS,
        }, format="json")
        assert resp.status_code == 400

    def test_filter_by_enabled(self, api, owner, tenant):
        Rule.objects.create(
            tenant=tenant, name="enabled-rule",
            conditions=SIMPLE_CONDITION, actions=SIMPLE_ACTIONS, enabled=True,
        )
        Rule.objects.create(
            tenant=tenant, name="disabled-rule",
            conditions=SIMPLE_CONDITION, actions=SIMPLE_ACTIONS, enabled=False,
        )
        token = _jwt(owner, tenant)
        api.credentials(HTTP_AUTHORIZATION=f"Bearer {token}")
        resp = api.get("/api/v1/rules/?enabled=true")
        assert resp.status_code == 200
        names = [r["name"] for r in resp.data]
        assert "enabled-rule" in names
        assert "disabled-rule" not in names
