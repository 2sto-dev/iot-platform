from rest_framework import viewsets
from rest_framework.decorators import api_view, permission_classes
from rest_framework.permissions import IsAuthenticated
from rest_framework.response import Response
from django.shortcuts import get_object_or_404
from .models import Client, Device
from .serializers import DeviceSerializer
from rest_framework_simplejwt.views import TokenObtainPairView
from .tokens import CustomTokenObtainPairSerializer


class DeviceViewSet(viewsets.ModelViewSet):
    """CRUD complet pentru Device"""
    queryset = Device.objects.all()
    serializer_class = DeviceSerializer

    def get_queryset(self):
        """Superuser vede toate device-urile, user normal doar pe ale lui"""
        user = self.request.user
        if not user.is_authenticated:
            # Dacă nu există JWT valid → returnăm listă goală, nu eroare
            return Device.objects.none()
        if user.is_superuser:
            return Device.objects.all()
        return Device.objects.filter(client=user)

class CustomTokenObtainPairView(TokenObtainPairView):
    serializer_class = CustomTokenObtainPairSerializer

@api_view(["GET"])
@permission_classes([IsAuthenticated])
def user_devices(request, username):
    """
    Returnează device-urile + topicurile aferente pentru un client specificat.
    - Superuser poate vedea device-urile oricui.
    - Un user normal vede doar propriile device-uri.
    """
    user = get_object_or_404(Client, username=username)

    if request.user != user and not request.user.is_superuser:
        return Response({"detail": "Not authorized"}, status=403)

    devices = Device.objects.filter(client=user)
    serializer = DeviceSerializer(devices, many=True)
    return Response(serializer.data)






