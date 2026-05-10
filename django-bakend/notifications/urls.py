from django.urls import path
from .views import (
    ChannelListCreateView,
    ChannelDetailView,
    ChannelTestView,
    EventListView,
    InternalNotifyView,
)

urlpatterns = [
    path("channels/", ChannelListCreateView.as_view(), name="channel-list"),
    path("channels/<int:pk>/", ChannelDetailView.as_view(), name="channel-detail"),
    path("channels/<int:pk>/test/", ChannelTestView.as_view(), name="channel-test"),
    path("events/", EventListView.as_view(), name="event-list"),
]

internal_urlpatterns = [
    path("notifications/trigger/", InternalNotifyView.as_view(), name="internal-notify"),
]
