from rest_framework_simplejwt.serializers import TokenObtainPairSerializer

class CustomTokenObtainPairSerializer(TokenObtainPairSerializer):
    @classmethod
    def get_token(cls, user):
        token = super().get_token(user)
        # includem username-ul în payload (pentru debug sau aplicații)
        token["username"] = user.username
        # adăugăm un issuer fix, pe care îl va folosi Kong pentru validare
        token["iss"] = "django"
        return token
