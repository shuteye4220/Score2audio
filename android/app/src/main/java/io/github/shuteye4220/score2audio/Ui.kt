package io.github.shuteye4220.score2audio

import android.app.Activity
import android.app.Application
import android.graphics.Typeface
import android.text.InputType
import android.view.View
import android.view.ViewGroup
import android.view.inputmethod.EditorInfo
import android.widget.AdapterView
import android.widget.ArrayAdapter
import android.widget.Button
import android.widget.CheckBox
import android.widget.EditText
import android.widget.LinearLayout
import android.widget.ScrollView
import android.widget.Spinner
import android.widget.TextView

class App : Application()

/**
 * Builds simple screens from standard Android controls, one below the
 * other, so screen readers and braille displays can move through them in
 * order.
 */
class Ui(private val a: Activity) {
    val column = LinearLayout(a).apply {
        orientation = LinearLayout.VERTICAL
        setPadding(dp(16), dp(8), dp(16), dp(24))
    }
    private val scroll = ScrollView(a).apply { addView(column) }

    fun dp(v: Int) = (v * a.resources.displayMetrics.density).toInt()

    fun show() = a.setContentView(scroll)

    private fun <T : View> add(v: T): T {
        column.addView(v, LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT))
        return v
    }

    fun heading(text: String): TextView = add(TextView(a).apply {
        this.text = text
        textSize = 22f
        setTypeface(typeface, Typeface.BOLD)
        isAccessibilityHeading = true
        setPadding(0, dp(18), 0, dp(4))
    })

    fun text(text: String): TextView = add(TextView(a).apply {
        this.text = text
        textSize = 18f
        setPadding(0, dp(4), 0, dp(4))
        setTextIsSelectable(false)
    })

    /** A line of text for messages; [say] shows it and has it spoken. */
    fun status(): TextView = text("").apply { visibility = View.GONE }

    fun button(text: String, onClick: () -> Unit): Button = add(Button(a).apply {
        this.text = text
        isAllCaps = false
        textSize = 18f
        gravity = android.view.Gravity.START or android.view.Gravity.CENTER_VERTICAL
        setOnClickListener { onClick() }
    })

    fun check(text: String, checked: Boolean, onChange: (Boolean) -> Unit): CheckBox = add(CheckBox(a).apply {
        this.text = text
        textSize = 18f
        isChecked = checked
        minHeight = dp(48)
        setOnCheckedChangeListener { _, c -> onChange(c) }
    })

    /** A labelled dropdown list. */
    fun choice(label: String, options: List<String>, selected: Int, onSelect: (Int) -> Unit): Spinner {
        val tv = text(label)
        val sp = Spinner(a, Spinner.MODE_DIALOG)
        sp.id = View.generateViewId()
        sp.prompt = label
        sp.adapter = ArrayAdapter(a, android.R.layout.simple_spinner_item, options).also {
            it.setDropDownViewResource(android.R.layout.simple_list_item_single_choice)
        }
        sp.setSelection(selected.coerceIn(0, maxOf(0, options.size - 1)), false)
        sp.minimumHeight = dp(48)
        tv.labelFor = sp.id
        var last = sp.selectedItemPosition
        sp.onItemSelectedListener = object : AdapterView.OnItemSelectedListener {
            override fun onItemSelected(parent: AdapterView<*>?, view: View?, position: Int, id: Long) {
                if (position != last) {
                    last = position
                    onSelect(position)
                }
            }
            override fun onNothingSelected(parent: AdapterView<*>?) {}
        }
        return add(sp)
    }

    /** A labelled text field; onDone runs when the user finishes editing. */
    fun field(label: String, value: String, number: Boolean, onDone: (String) -> Unit): EditText {
        val tv = text(label)
        val et = EditText(a)
        et.id = View.generateViewId()
        et.setText(value)
        et.textSize = 18f
        et.inputType = if (number) InputType.TYPE_CLASS_NUMBER else InputType.TYPE_CLASS_TEXT or InputType.TYPE_TEXT_FLAG_CAP_SENTENCES
        et.imeOptions = EditorInfo.IME_ACTION_DONE
        et.isSingleLine = true
        et.minHeight = dp(48)
        tv.labelFor = et.id
        et.setOnEditorActionListener { v, actionId, _ ->
            if (actionId == EditorInfo.IME_ACTION_DONE) onDone(v.text.toString())
            false
        }
        et.setOnFocusChangeListener { v, has -> if (!has) onDone((v as EditText).text.toString()) }
        return add(et)
    }
}

/** Shows a message in a status line and has screen readers speak it. */
fun TextView.say(message: String) {
    visibility = if (message.isEmpty()) View.GONE else View.VISIBLE
    text = message
    if (message.isNotEmpty()) announceForAccessibility(message)
}
