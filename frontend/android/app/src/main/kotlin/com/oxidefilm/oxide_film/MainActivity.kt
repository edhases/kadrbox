package com.oxidefilm.oxide_film

import android.app.PictureInPictureParams
import android.content.res.Configuration
import android.os.Build
import android.util.Rational
import io.flutter.embedding.android.FlutterActivity
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.MethodChannel

class MainActivity : FlutterActivity() {
    private val CHANNEL = "com.oxidefilm.oxide_film/pip"

    override fun configureFlutterEngine(flutterEngine: FlutterEngine) {
        super.configureFlutterEngine(flutterEngine)
        
        val messenger = flutterEngine.dartExecutor.binaryMessenger
        MethodChannel(messenger, CHANNEL).setMethodCallHandler { call, result ->
            // Logging for debugging
            android.util.Log.d("MainActivity", "Method called: ${call.method}")
            
            if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) {
                result.success(null)
                return@setMethodCallHandler
            }

            when (call.method) {
                "enterPiP" -> {
                    val aspectRatio = Rational(16, 9)
                    val params = PictureInPictureParams.Builder()
                        .setAspectRatio(aspectRatio)
                        .build()
                    try {
                        enterPictureInPictureMode(params)
                        result.success(true)
                    } catch (e: Exception) {
                        result.error("PIP_ERROR", e.message, null)
                    }
                }
                "setAutoEnterEnabled" -> {
                    if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
                        val enabled = call.arguments as Boolean
                        val aspectRatio = Rational(16, 9)
                        val params = PictureInPictureParams.Builder()
                            .setAspectRatio(aspectRatio)
                            .setAutoEnterEnabled(enabled)
                            .build()
                        setPictureInPictureParams(params)
                        result.success(true)
                    } else {
                        result.success(false)
                    }
                }
                else -> {
                    result.notImplemented()
                }
            }
        }
    }

    override fun onPictureInPictureModeChanged(isInPictureInPictureMode: Boolean, newConfig: Configuration) {
        super.onPictureInPictureModeChanged(isInPictureInPictureMode, newConfig)
        flutterEngine?.dartExecutor?.binaryMessenger?.let {
            MethodChannel(it, CHANNEL).invokeMethod("onPiPStatusChanged", isInPictureInPictureMode)
        }
    }
}
