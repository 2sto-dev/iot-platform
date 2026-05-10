from django.urls import path
from .views import (
    RuleListCreateView,
    RuleDetailView,
    RuleToggleView,
    RuleExecutionListView,
    RuleExecutionAllView,
    InternalRuleListView,
    InternalRuleLogView,
)

urlpatterns = [
    # Public (JWT/ApiKey)
    path("", RuleListCreateView.as_view(), name="rule-list"),
    path("<int:pk>/", RuleDetailView.as_view(), name="rule-detail"),
    path("<int:pk>/toggle/", RuleToggleView.as_view(), name="rule-toggle"),
    path("<int:pk>/executions/", RuleExecutionListView.as_view(), name="rule-executions"),
    path("executions/", RuleExecutionAllView.as_view(), name="rule-executions-all"),
]

internal_urlpatterns = [
    path("rules/", InternalRuleListView.as_view(), name="internal-rules"),
    path("rules/log/", InternalRuleLogView.as_view(), name="internal-rule-log"),
]
