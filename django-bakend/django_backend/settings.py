from pathlib import Path
from decouple import config, Csv
from datetime import timedelta

BASE_DIR = Path(__file__).resolve().parent.parent

STATIC_URL = "/static/"
STATIC_ROOT = BASE_DIR / "staticfiles"

# ⚙️ Security
SECRET_KEY = config("DJANGO_SECRET_KEY")
DEBUG = config("DJANGO_DEBUG", cast=bool)
ALLOWED_HOSTS = config("DJANGO_ALLOWED_HOSTS", cast=Csv())

# 📦 Apps
INSTALLED_APPS = [
    "corsheaders",
    "django.contrib.admin",
    "django.contrib.auth",
    "django.contrib.contenttypes",
    "django.contrib.sessions",
    "django.contrib.messages",
    "django.contrib.staticfiles",
    "rest_framework",
    "drf_spectacular",
    "clients",
    "tenants",
    "provisioning",
    "ota",
    "audit",
    "api_keys",
    "rules",
    "notifications",
]

MIDDLEWARE = [
    "corsheaders.middleware.CorsMiddleware",
    "django.middleware.security.SecurityMiddleware",
    "django.contrib.sessions.middleware.SessionMiddleware",
    "django.middleware.common.CommonMiddleware",
    "django.middleware.csrf.CsrfViewMiddleware",
    "django.contrib.auth.middleware.AuthenticationMiddleware",
    "django.contrib.messages.middleware.MessageMiddleware",
    "django.middleware.clickjacking.XFrameOptionsMiddleware",
    "tenants.middleware.TenantMiddleware",
    "audit.middleware.AuditMiddleware",
]

ROOT_URLCONF = "django_backend.urls"

TEMPLATES = [
    {
        "BACKEND": "django.template.backends.django.DjangoTemplates",
        "DIRS": [],
        "APP_DIRS": True,
        "OPTIONS": {
            "context_processors": [
                "django.template.context_processors.debug",
                "django.template.context_processors.request",
                "django.contrib.auth.context_processors.auth",
                "django.contrib.messages.context_processors.messages",
            ],
        },
    },
]

WSGI_APPLICATION = "django_backend.wsgi.application"

# 💾 Database – doar din .env
DATABASES = {
    "default": {
        "ENGINE": "django.db.backends.mysql",
        "NAME": config("DB_NAME"),
        "USER": config("DB_USER"),
        "PASSWORD": config("DB_PASSWORD"),
        "HOST": config("DB_HOST"),
        "PORT": config("DB_PORT"),
    }
}

# 🔐 Password hashers — bcrypt pentru mqtt_password_hash pe Device (Faza 3.1)
PASSWORD_HASHERS = [
    "django.contrib.auth.hashers.PBKDF2PasswordHasher",  # default pt. user passwords
    "django.contrib.auth.hashers.BCryptSHA256PasswordHasher",  # pt. device MQTT passwords
]

# 🔐 Password validation
AUTH_PASSWORD_VALIDATORS = [
    {
        "NAME": "django.contrib.auth.password_validation.UserAttributeSimilarityValidator"
    },
    {"NAME": "django.contrib.auth.password_validation.MinimumLengthValidator"},
    {"NAME": "django.contrib.auth.password_validation.CommonPasswordValidator"},
    {"NAME": "django.contrib.auth.password_validation.NumericPasswordValidator"},
]

# 👤 Custom user model
AUTH_USER_MODEL = "clients.Client"

# 🔑 REST Framework
REST_FRAMEWORK = {
    "DEFAULT_AUTHENTICATION_CLASSES": (
        "rest_framework_simplejwt.authentication.JWTAuthentication",
        "api_keys.authentication.APIKeyAuthentication",
    ),
    "DEFAULT_PERMISSION_CLASSES": ("rest_framework.permissions.IsAuthenticated",),
    "DEFAULT_SCHEMA_CLASS": "drf_spectacular.openapi.AutoSchema",
}

# 📖 OpenAPI spec (drf-spectacular)
SPECTACULAR_SETTINGS = {
    "TITLE": "IoT Platform API",
    "DESCRIPTION": "REST API for IoT device management, telemetry, and control.",
    "VERSION": "1.0.0",
    "SERVE_INCLUDE_SCHEMA": False,
}

# 🌍 Internationalization
LANGUAGE_CODE = "en-us"
TIME_ZONE = "UTC"
USE_I18N = True
USE_TZ = True

# 📂 Static files
STATIC_URL = "static/"

# 🔑 Default primary key
DEFAULT_AUTO_FIELD = "django.db.models.BigAutoField"


SIMPLE_JWT = {
    "SIGNING_KEY": config("JWT_SECRET"),
    "ALGORITHM": "HS256",
    "ACCESS_TOKEN_LIFETIME": timedelta(minutes=100),
    "REFRESH_TOKEN_LIFETIME": timedelta(days=7),
}

# Kill-switch pentru întreg flow-ul multi-tenant (TenantMiddleware + queryset filter).
# La False: middleware devine no-op și viewset-urile permit acces ne-tenant-scoped.
# Util pentru rollback de urgență fără rollback de migrare DB.
MULTI_TENANT_ENABLED = config("MULTI_TENANT_ENABLED", default=True, cast=bool)

# Faza 3.4: MQTT broker pentru shadow delta push (retained messages).
# Format: "tcp://host:port" sau "host:port". Gol = publisher dezactivat (no-op).
MQTT_BROKER = config("MQTT_BROKER", default="")
MQTT_SERVICE_USER = config("DJANGO_SERVICE_USER", default="")
MQTT_SERVICE_PASS = config("DJANGO_SERVICE_PASS", default="")

# Faza 2.4: Redis pentru cache device→tenant invalidation pub/sub.
# Format URL: redis://[:password]@host:port/db (vezi clients/signals.py).
# Opțional: dacă lipsește, signals devin no-op și Go-ul cade pe fallback Django per-message.
REDIS_URL = config("REDIS_URL", default="")

# CORS — permite dashboard React în dev (Vite pe :5173)
CORS_ALLOWED_ORIGINS = config(
    "CORS_ALLOWED_ORIGINS",
    default="http://localhost:5173",
    cast=Csv(),
)
CORS_ALLOW_CREDENTIALS = True
