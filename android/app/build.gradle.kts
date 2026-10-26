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
            // Published builds are signed with the project's private key,
            // which GitHub supplies from the repository secrets (see
            // README.md). It is never kept in the repository.
            System.getenv("SIGNING_STORE_FILE")?.let { store ->
                storeFile = file(store)
                storePassword = System.getenv("SIGNING_STORE_PASSWORD")
                keyAlias = System.getenv("SIGNING_KEY_ALIAS") ?: "score2audio"
                keyPassword = System.getenv("SIGNING_KEY_PASSWORD") ?: storePassword
                storeType = "pkcs12"
            }
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            // Without the private key (a build on your own computer), use
            // Android's debug key so the app can still be built and tried.
            signingConfig = if (System.getenv("SIGNING_STORE_FILE") != null) signingConfigs.getByName("release")
                else signingConfigs.getByName("debug")
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
