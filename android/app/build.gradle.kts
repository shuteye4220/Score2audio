plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

// The version comes from the git tag when GitHub builds a release.
val appVersionName = (project.findProperty("appVersion") as String?) ?: "1.0.0-dev"
val appVersionCode = (project.findProperty("appVersionCode") as String?)?.toIntOrNull() ?: 1

android {
    namespace = "io.github.shuteye4220.score2audio"
    compileSdk = 35

    defaultConfig {
        applicationId = "io.github.shuteye4220.score2audio"
        minSdk = 29
        targetSdk = 34
        versionCode = appVersionCode
        versionName = appVersionName
    }

    signingConfigs {
        create("release") {
            // A private key can be supplied through environment variables
            // (GitHub secrets); otherwise the project's own key is used, so
            // every build can update the previous one.
            val envStore = System.getenv("SIGNING_STORE_FILE")
            storeFile = if (envStore != null) file(envStore) else rootProject.file("score2audio.p12")
            storePassword = System.getenv("SIGNING_STORE_PASSWORD") ?: "score2audio"
            keyAlias = System.getenv("SIGNING_KEY_ALIAS") ?: "score2audio"
            keyPassword = System.getenv("SIGNING_KEY_PASSWORD") ?: "score2audio"
            storeType = "pkcs12"
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            signingConfig = signingConfigs.getByName("release")
        }
        debug {
            signingConfig = signingConfigs.getByName("release")
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlinOptions {
        jvmTarget = "17"
    }
    lint {
        abortOnError = false
    }
}

dependencies {
}
